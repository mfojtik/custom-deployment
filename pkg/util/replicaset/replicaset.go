/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package replicaset

import (
	"fmt"
	"time"

	podutil "github.com/mfojtik/custom-deployment/pkg/util/pod"
	typed "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/labels"
	errorsutil "k8s.io/client-go/1.5/pkg/util/errors"
	labelsutil "k8s.io/client-go/1.5/pkg/util/labels"
	"k8s.io/client-go/1.5/pkg/util/wait"
)

// TODO: use client library instead when it starts to support update retries
//       see https://github.com/kubernetes/kubernetes/issues/21479
type updateRSFunc func(rs *v1beta1.ReplicaSet) error

// UpdateRSWithRetries updates a RS with given applyUpdate function. Note that RS not found error is ignored.
// The returned bool value can be used to tell if the RS is actually updated.
func UpdateRSWithRetries(rsClient typed.ReplicaSetInterface, rs *v1beta1.ReplicaSet, applyUpdate updateRSFunc) (*v1beta1.ReplicaSet, bool, error) {
	var err error
	var rsUpdated bool
	oldRs := rs
	if err = wait.Poll(10*time.Millisecond, 1*time.Minute, func() (bool, error) {
		rs, err = rsClient.Get(oldRs.Name)
		if err != nil {
			return false, err
		}
		// Apply the update, then attempt to push it to the apiserver.
		if err = applyUpdate(rs); err != nil {
			return false, err
		}
		if rs, err = rsClient.Update(rs); err == nil {
			// Update successful.
			return true, nil
		}
		// TODO: don't retry on perm-failed errors and handle them gracefully
		// Update could have failed due to conflict error. Try again.
		return false, nil
	}); err == nil {
		// When there's no error, we've updated this RS.
		rsUpdated = true
	}

	// Handle returned error from wait poll
	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("timed out trying to update RS: %#v", oldRs)
	}
	// Ignore the RS not found error, but the RS isn't updated.
	if errors.IsNotFound(err) {
		err = nil
	}
	// Ignore the precondition violated error, but the RS isn't updated.
	if err == errorsutil.ErrPreconditionViolated {
		err = nil
	}

	// If the error is non-nil the returned RS cannot be trusted; if rsUpdated is false, the contoller isn't updated;
	// if the error is nil and rsUpdated is true, the returned RS contains the applied update.
	return rs, rsUpdated, err
}

// GetPodTemplateSpecHash returns the pod template hash of a ReplicaSet's pod template space
func GetPodTemplateSpecHash(rs *v1beta1.ReplicaSet) string {
	meta := rs.Spec.Template.ObjectMeta
	meta.Labels = labelsutil.CloneAndRemoveLabel(meta.Labels, extensions.DefaultDeploymentUniqueLabelKey)
	return fmt.Sprintf("%d", podutil.GetPodTemplateSpecHash(v1.PodTemplateSpec{
		ObjectMeta: meta,
		Spec:       rs.Spec.Template.Spec,
	}))
}

// MatchingPodsFunc returns a filter function for pods with matching labels
func MatchingPodsFunc(rs *v1beta1.ReplicaSet) (func(v1.Pod) bool, error) {
	if rs == nil {
		return nil, nil
	}
	selector, err := unversioned.LabelSelectorAsSelector(&unversioned.LabelSelector{MatchLabels: rs.Spec.Selector.MatchLabels})
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: %v", err)
	}
	return func(pod v1.Pod) bool {
		podLabelsSelector := labels.Set(pod.ObjectMeta.Labels)
		return selector.Matches(podLabelsSelector)
	}, nil
}
