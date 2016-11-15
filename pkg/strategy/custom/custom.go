package custom

import (
	"fmt"
	"log"
	"strconv"

	"github.com/mfojtik/custom-deployment/pkg/informers"
	"github.com/mfojtik/custom-deployment/pkg/util/conversion"
	deploymentutil "github.com/mfojtik/custom-deployment/pkg/util/deployments"
	podutil "github.com/mfojtik/custom-deployment/pkg/util/pod"
	rsutil "github.com/mfojtik/custom-deployment/pkg/util/replicaset"
	typedCore "k8s.io/client-go/1.5/kubernetes/typed/core/v1"
	typed "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	utilerrors "k8s.io/client-go/1.5/pkg/util/errors"
	labelsutil "k8s.io/client-go/1.5/pkg/util/labels"
	"k8s.io/client-go/1.5/tools/cache"
)

type Strategy struct {
	extClient  typed.ExtensionsInterface
	coreClient typedCore.CoreInterface

	replicaSetLister *cache.StoreToReplicaSetLister
}

func NewStrategy(rsInformer informers.ReplicaSetInformer, extClient typed.ExtensionsInterface, coreClient typedCore.CoreInterface) *Strategy {
	strategy := &Strategy{
		extClient:        extClient,
		coreClient:       coreClient,
		replicaSetLister: rsInformer.Lister(),
	}
	return strategy
}

func (s *Strategy) Rollout(deployment *extensions.Deployment) error {
	newRS, oldRSs, err := s.getAllReplicaSetsAndSyncRevision(deployment, false)
	if err != nil {
		return err
	}
	//allRSs := append(oldRSs, newRS)
	activeOldRSs := deploymentutil.FilterActiveReplicaSets(oldRSs)
	log.Printf("scaling down %#+v", activeOldRSs)
	log.Printf("scaling up %#+v", newRS)
	return nil
}

func (s *Strategy) getAllReplicaSetsAndSyncRevision(deployment *extensions.Deployment, createIfNotExisted bool) (*extensions.ReplicaSet, []*extensions.ReplicaSet, error) {
	rsList, podList, err := s.rsAndPodsWithHashKeySynced(deployment)
	if err != nil {
		return nil, nil, fmt.Errorf("error labeling replica sets and pods with pod-template-hash: %v", err)
	}
	_, allOldRSs, err := deploymentutil.FindOldReplicaSets(deployment, rsList, podList)
	if err != nil {
		return nil, nil, err
	}

	// Get new replica set with the updated revision number
	newRS, err := s.getNewReplicaSet(deployment, rsList, allOldRSs, createIfNotExisted)
	if err != nil {
		return nil, nil, err
	}

	return newRS, allOldRSs, nil
}

func (s *Strategy) rsAndPodsWithHashKeySynced(deployment *extensions.Deployment) ([]*extensions.ReplicaSet, *api.PodList, error) {
	rsList, err := s.listDeploymentReplicaSets(deployment)
	if err != nil {
		return nil, nil, err
	}
	syncedRSList := []*extensions.ReplicaSet{}
	for _, rs := range rsList {
		// Add pod-template-hash information if it's not in the RS.
		// Otherwise, new RS produced by Deployment will overlap with pre-existing ones
		// that aren't constrained by the pod-template-hash.
		syncedRS, err := s.addHashKeyToRSAndPods(&rs)
		if err != nil {
			return nil, nil, err
		}
		syncedRSList = append(syncedRSList, syncedRS)
	}
	syncedPodList, err := s.listPods(deployment)
	if err != nil {
		return nil, nil, err
	}
	return syncedRSList, syncedPodList, nil
}

func (s *Strategy) addHashKeyToRSAndPods(rs *extensions.ReplicaSet) (updatedRS *extensions.ReplicaSet, err error) {
	objCopy, err := api.Scheme.Copy(rs)
	if err != nil {
		return nil, err
	}
	updatedRS = objCopy.(*extensions.ReplicaSet)

	// If the rs already has the new hash label in its selector, it's done syncing
	if labelsutil.SelectorHasLabel(&unversioned.LabelSelector{MatchLabels: rs.Spec.Selector.MatchLabels}, extensions.DefaultDeploymentUniqueLabelKey) {
		return
	}
	namespace := rs.Namespace
	hash := rsutil.GetPodTemplateSpecHash(rs)
	rsUpdated := false
	// 1. Add hash template label to the rs. This ensures that any newly created pods will have the new label.
	updatedRS, rsUpdated, err = rsutil.UpdateRSWithRetries(s.extClient.ReplicaSets(namespace), updatedRS,
		func(updated *extensions.ReplicaSet) error {
			// Precondition: the RS doesn't contain the new hash in its pod template label.
			if updated.Spec.Template.Labels[extensions.DefaultDeploymentUniqueLabelKey] == hash {
				return utilerrors.ErrPreconditionViolated
			}
			updated.Spec.Template.Labels = labelsutil.AddLabel(updated.Spec.Template.Labels, extensions.DefaultDeploymentUniqueLabelKey, hash)
			return nil
		})
	if err != nil {
		return nil, fmt.Errorf("error updating %s %s/%s pod template label with template hash: %v", updatedRS.Kind, updatedRS.Namespace, updatedRS.Name, err)
	}
	if !rsUpdated {
		// If RS wasn't updated but didn't return error in step 1, we've hit a RS not found error.
		// Return here and retry in the next sync loop.
		return rs, nil
	}
	// Make sure rs pod template is updated so that it won't create pods without the new label (orphaned pods).
	if updatedRS.Generation > updatedRS.Status.ObservedGeneration {
		if err = deploymentutil.WaitForReplicaSetUpdated(s.extClient, updatedRS.Generation, namespace, updatedRS.Name); err != nil {
			return nil, fmt.Errorf("error waiting for %s %s/%s generation %d observed by controller: %v", updatedRS.Kind, updatedRS.Namespace, updatedRS.Name, updatedRS.Generation, err)
		}
	}

	// 2. Update all pods managed by the rs to have the new hash label, so they will be correctly adopted.
	selector, err := unversioned.LabelSelectorAsSelector(&unversioned.LabelSelector{MatchLabels: updatedRS.Spec.Selector.MatchLabels})
	if err != nil {
		return nil, fmt.Errorf("error in converting selector to label selector for replica set %s: %s", updatedRS.Name, err)
	}
	options := api.ListOptions{LabelSelector: selector}
	// TODO: Replace with lister
	externalPodList, err := s.coreClient.Pods(namespace).List(options)
	if err != nil {
		return nil, fmt.Errorf("error in getting pod list for namespace %s and list options %+v: %s", namespace, options, err)
	}
	podList := &api.PodList{}
	if err := v1.Convert_v1_PodList_To_api_PodList(externalPodList, podList, nil); err != nil {
		return nil, err
	}
	allPodsLabeled := false
	if allPodsLabeled, err = deploymentutil.LabelPodsWithHash(podList, updatedRS, s.coreClient, namespace, hash); err != nil {
		return nil, fmt.Errorf("error in adding template hash label %s to pods %+v: %s", hash, podList, err)
	}
	// If not all pods are labeled but didn't return error in step 2, we've hit at least one pod not found error.
	// Return here and retry in the next sync loop.
	if !allPodsLabeled {
		return updatedRS, nil
	}

	// We need to wait for the replicaset controller to observe the pods being
	// labeled with pod template hash. Because previously we've called
	// WaitForReplicaSetUpdated, the replicaset controller should have dropped
	// FullyLabeledReplicas to 0 already, we only need to wait it to increase
	// back to the number of replicas in the spec.
	if err = deploymentutil.WaitForPodsHashPopulated(s.extClient, updatedRS.Generation, namespace, updatedRS.Name); err != nil {
		return nil, fmt.Errorf("%s %s/%s: error waiting for replicaset controller to observe pods being labeled with template hash: %v", updatedRS.Kind, updatedRS.Namespace, updatedRS.Name, err)
	}

	// 3. Update rs label and selector to include the new hash label
	// Copy the old selector, so that we can scrub out any orphaned pods
	if updatedRS, rsUpdated, err = rsutil.UpdateRSWithRetries(s.extClient.ReplicaSets(namespace), updatedRS,
		func(updated *extensions.ReplicaSet) error {
			// Precondition: the RS doesn't contain the new hash in its label or selector.
			if updated.Labels[extensions.DefaultDeploymentUniqueLabelKey] == hash && updated.Spec.Selector.MatchLabels[extensions.DefaultDeploymentUniqueLabelKey] == hash {
				return utilerrors.ErrPreconditionViolated
			}
			updated.Labels = labelsutil.AddLabel(updated.Labels, extensions.DefaultDeploymentUniqueLabelKey, hash)
			updated.Spec.Selector = labelsutil.AddLabelToSelector(updated.Spec.Selector, extensions.DefaultDeploymentUniqueLabelKey, hash)
			return nil
		}); err != nil {
		return nil, fmt.Errorf("error updating %s %s/%s label and selector with template hash: %v", updatedRS.Kind, updatedRS.Namespace, updatedRS.Name, err)
	}

	return updatedRS, nil
}

func (s *Strategy) listDeploymentReplicaSets(deployment *extensions.Deployment) ([]extensions.ReplicaSet, error) {
	selector, err := unversioned.LabelSelectorAsSelector(&unversioned.LabelSelector{MatchLabels: deployment.Spec.Selector.MatchLabels})
	if err != nil {
		return nil, err
	}
	result, err := s.extClient.ReplicaSets(deployment.Namespace).List(api.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	// TODO: We can't use lister here because it panics as the lister cast the
	// v1beta.ReplicaSet to internal (but the lister client is external...)
	out := []extensions.ReplicaSet{}
	for _, r := range result.Items {
		internal := conversion.ReplicaSetToInternal(&r)
		out = append(out, *internal)
	}
	return out, nil
}

func (s *Strategy) listPods(deployment *extensions.Deployment) (*api.PodList, error) {
	selector, err := unversioned.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}

	externalPodList, err := s.coreClient.Pods(deployment.Namespace).List(api.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, err
	}
	podList := &api.PodList{}
	if err := v1.Convert_v1_PodList_To_api_PodList(externalPodList, podList, nil); err != nil {
		return nil, err
	}
	return podList, nil
}

// Returns a replica set that matches the intent of the given deployment. Returns nil if the new replica set doesn't exist yet.
// 1. Get existing new RS (the RS that the given deployment targets, whose pod template is the same as deployment's).
// 2. If there's existing new RS, update its revision number if it's smaller than (maxOldRevision + 1), where maxOldRevision is the max revision number among all old RSes.
// 3. If there's no existing new RS and createIfNotExisted is true, create one with appropriate revision number (maxOldRevision + 1) and replicas.
// Note that the pod-template-hash will be added to adopted RSes and pods.
func (s *Strategy) getNewReplicaSet(deployment *extensions.Deployment, rsList, oldRSs []*extensions.ReplicaSet, createIfNotExisted bool) (*extensions.ReplicaSet, error) {
	existingNewRS, err := deploymentutil.FindNewReplicaSet(deployment, rsList)
	if err != nil {
		return nil, err
	}

	// Calculate the max revision number among all old RSes
	maxOldRevision := deploymentutil.MaxRevision(oldRSs)
	// Calculate revision number for this new replica set
	newRevision := strconv.FormatInt(maxOldRevision+1, 10)

	// Latest replica set exists. We need to sync its annotations (includes copying all but
	// annotationsToSkip from the parent deployment, and update revision, desiredReplicas,
	// and maxReplicas) and also update the revision annotation in the deployment with the
	// latest revision.
	if existingNewRS != nil {
		objCopy, err := api.Scheme.Copy(existingNewRS)
		if err != nil {
			return nil, err
		}
		rsCopy := objCopy.(*extensions.ReplicaSet)

		// Set existing new replica set's annotation
		annotationsUpdated := deploymentutil.SetNewReplicaSetAnnotations(deployment, rsCopy, newRevision, true)
		if annotationsUpdated {
			result, err := s.extClient.ReplicaSets(rsCopy.ObjectMeta.Namespace).Update(conversion.ReplicaSetToExternal(rsCopy))
			return conversion.ReplicaSetToInternal(result), err
		}

		updateConditions := deploymentutil.SetDeploymentRevision(deployment, newRevision)
		// If no other Progressing condition has been recorded and we need to estimate the progress
		// of this deployment then it is likely that old users started caring about progress. In that
		// case we need to take into account the first time we noticed their new replica set.
		if updateConditions {
			if _, err = s.extClient.Deployments(deployment.Namespace).UpdateStatus(conversion.DeploymentToExternal(deployment)); err != nil {
				return nil, err
			}
		}
		return rsCopy, nil
	}

	if !createIfNotExisted {
		return nil, nil
	}

	// new ReplicaSet does not exist, create one.
	namespace := deployment.Namespace
	podTemplateSpecHash := podutil.GetPodTemplateSpecHash(deployment.Spec.Template)
	newRSTemplate := deploymentutil.GetNewReplicaSetTemplate(deployment)
	// Add podTemplateHash label to selector.
	newRSSelector := labelsutil.CloneSelectorAndAddLabel(deployment.Spec.Selector, extensions.DefaultDeploymentUniqueLabelKey, podTemplateSpecHash)

	// Create new ReplicaSet
	newRS := extensions.ReplicaSet{
		ObjectMeta: api.ObjectMeta{
			// Make the name deterministic, to ensure idempotence
			Name:      deployment.Name + "-" + fmt.Sprintf("%d", podTemplateSpecHash),
			Namespace: namespace,
		},
		Spec: extensions.ReplicaSetSpec{
			Replicas: 0,
			Selector: newRSSelector,
			Template: newRSTemplate,
		},
	}
	allRSs := append(oldRSs, &newRS)
	newReplicasCount, err := deploymentutil.NewRSNewReplicas(deployment, allRSs, &newRS)
	if err != nil {
		return nil, err
	}

	newRS.Spec.Replicas = newReplicasCount
	// Set new replica set's annotation
	deploymentutil.SetNewReplicaSetAnnotations(deployment, &newRS, newRevision, false)
	createdRS, err := s.extClient.ReplicaSets(namespace).Create(conversion.ReplicaSetToExternal(&newRS))
	switch {
	// We may end up hitting this due to a slow cache or a fast resync of the deployment.
	case errors.IsAlreadyExists(err):
		return s.replicaSetLister.ReplicaSets(namespace).Get(newRS.Name)
	case err != nil:
		return nil, err
	}

	deploymentutil.SetDeploymentRevision(deployment, newRevision)
	_, err = s.extClient.Deployments(deployment.Namespace).UpdateStatus(conversion.DeploymentToExternal(deployment))
	return conversion.ReplicaSetToInternal(createdRS), err
}
