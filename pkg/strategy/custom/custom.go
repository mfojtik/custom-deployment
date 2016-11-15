package custom

import (
	"fmt"

	"github.com/mfojtik/custom-deployment/pkg/informers"
	deploymentutil "github.com/mfojtik/custom-deployment/pkg/util/deployments"
	rsutil "github.com/mfojtik/custom-deployment/pkg/util/replicaset"
	typedCore "k8s.io/client-go/1.5/kubernetes/typed/core/v1"
	typed "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
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

func (s *Strategy) Rollout(newReplicaSet, oldReplicaSet *v1beta1.ReplicaSet, deployment *v1beta1.Deployment) error {
	return nil
}

func (s *Strategy) rsAndPodsWithHashKeySynced(deployment *extensions.Deployment) ([]*extensions.ReplicaSet, *api.PodList, error) {
	rsList, err := listDeploymentReplicaSets(deployment)
	if err != nil {
		return nil, nil, err
	}
	syncedRSList := []*extensions.ReplicaSet{}
	for _, rs := range rsList {
		// Add pod-template-hash information if it's not in the RS.
		// Otherwise, new RS produced by Deployment will overlap with pre-existing ones
		// that aren't constrained by the pod-template-hash.
		syncedRS, err := s.addHashKeyToRSAndPods(rs)
		if err != nil {
			return nil, nil, err
		}
		syncedRSList = append(syncedRSList, syncedRS)
	}
	syncedPodList, err := dc.listPods(deployment)
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
	if labelsutil.SelectorHasLabel(rs.Spec.Selector, extensions.DefaultDeploymentUniqueLabelKey) {
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
	selector, err := unversioned.LabelSelectorAsSelector(updatedRS.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("error in converting selector to label selector for replica set %s: %s", updatedRS.Name, err)
	}
	options := api.ListOptions{LabelSelector: selector}
	// TODO: Replace with lister
	pods, err := s.coreClient.Pods(namespace).List(options.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("error in getting pod list for namespace %s and list options %+v: %s", namespace, options, err)
	}
	podList := api.PodList{Items: make([]api.Pod, 0, len(pods))}
	for i := range pods {
		podList.Items = append(podList.Items, *pods[i])
	}
	allPodsLabeled := false
	if allPodsLabeled, err = deploymentutil.LabelPodsWithHash(&podList, updatedRS, s.coreClient, namespace, hash); err != nil {
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
	if err = deploymentutil.WaitForPodsHashPopulated(s.coreClient, updatedRS.Generation, namespace, updatedRS.Name); err != nil {
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

func (s *Strategy) listDeploymentReplicaSets(deployment *v1beta1.Deployment) ([]*extensions.ReplicaSet, error) {
	selector, err := unversioned.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return s.replicaSetLister.ReplicaSets(deployment.Namespace).List(api.ListOptions{
		LabelSelector: selector,
	})
}
