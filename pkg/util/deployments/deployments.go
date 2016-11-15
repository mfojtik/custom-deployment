package deployments

import (
	"fmt"
	"time"

	podutil "github.com/mfojtik/custom-deployment/pkg/util/pod"
	typedCore "k8s.io/client-go/1.5/kubernetes/typed/core/v1"
	typed "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/util/errors"
	labelsutil "k8s.io/client-go/1.5/pkg/util/labels"
	"k8s.io/client-go/1.5/pkg/util/wait"
)

const SelectorUpdateAnnotation = "deployment.kubernetes.io/selector-updated-at"

func DeploymentDeepCopy(deployment *v1beta1.Deployment) (*v1beta1.Deployment, error) {
	objCopy, err := api.Scheme.DeepCopy(deployment)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*v1beta1.Deployment)
	if !ok {
		return nil, fmt.Errorf("expected Deployment, got %#v", objCopy)
	}
	return copied, nil
}

// LastSelectorUpdate returns the last time given deployment's selector is updated
func LastSelectorUpdate(d *v1beta1.Deployment) unversioned.Time {
	t := d.Annotations[SelectorUpdateAnnotation]
	if len(t) > 0 {
		parsedTime, err := time.Parse(t, time.RFC3339)
		// If failed to parse the time, use creation timestamp instead
		if err != nil {
			return d.CreationTimestamp
		}
		return unversioned.Time{Time: parsedTime}
	}
	// If it's never updated, use creation timestamp instead
	return d.CreationTimestamp
}

// BySelectorLastUpdateTime sorts a list of deployments by the last update time of their selector,
// first using their creation timestamp and then their names as a tie breaker.
type BySelectorLastUpdateTime []*v1beta1.Deployment

func (o BySelectorLastUpdateTime) Len() int      { return len(o) }
func (o BySelectorLastUpdateTime) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o BySelectorLastUpdateTime) Less(i, j int) bool {
	ti, tj := LastSelectorUpdate(o[i]), LastSelectorUpdate(o[j])
	if ti.Equal(tj) {
		if o[i].CreationTimestamp.Equal(o[j].CreationTimestamp) {
			return o[i].Name < o[j].Name
		}
		return o[i].CreationTimestamp.Before(o[j].CreationTimestamp)
	}
	return ti.Before(tj)
}

// WaitForReplicaSetUpdated polls the replica set until it is updated.
func WaitForReplicaSetUpdated(c typed.ExtensionsInterface, desiredGeneration int64, namespace, name string) error {
	return wait.Poll(10*time.Millisecond, 1*time.Minute, func() (bool, error) {
		rs, err := c.ReplicaSets(namespace).Get(name)
		if err != nil {
			return false, err
		}
		return rs.Status.ObservedGeneration >= desiredGeneration, nil
	})
}

// LabelPodsWithHash labels all pods in the given podList with the new hash label.
// The returned bool value can be used to tell if all pods are actually labeled.
func LabelPodsWithHash(podList *v1.PodList, rs *extensions.ReplicaSet, c typedCore.CoreInterface, namespace, hash string) (bool, error) {
	allPodsLabeled := true
	for _, pod := range podList.Items {
		// Only label the pod that doesn't already have the new hash
		if pod.Labels[extensions.DefaultDeploymentUniqueLabelKey] != hash {
			if _, podUpdated, err := podutil.UpdatePodWithRetries(c.Pods(namespace), &pod,
				func(podToUpdate *v1.Pod) error {
					// Precondition: the pod doesn't contain the new hash in its label.
					if podToUpdate.Labels[extensions.DefaultDeploymentUniqueLabelKey] == hash {
						return errors.ErrPreconditionViolated
					}
					podToUpdate.Labels = labelsutil.AddLabel(podToUpdate.Labels, extensions.DefaultDeploymentUniqueLabelKey, hash)
					return nil
				}); err != nil {
				return false, fmt.Errorf("error in adding template hash label %s to pod %+v: %s", hash, pod, err)
			} else if podUpdated {
			} else {
				// If the pod wasn't updated but didn't return error when we try to update it, we've hit "pod not found" or "precondition violated" error.
				// Then we can't say all pods are labeled
				allPodsLabeled = false
			}
		}
	}
	return allPodsLabeled, nil
}
