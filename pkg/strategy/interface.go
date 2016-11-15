package strategy

import "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"

type Interface interface {
	Rollout(newReplicaSet, oldReplicaSet v1beta1.ReplicaSet, deployment v1beta1.Deployment) error
}
