package strategy

import (
	"k8s.io/client-go/1.5/pkg/apis/extensions"
)

type Interface interface {
	Rollout(newReplicaSet, oldReplicaSet *extensions.ReplicaSet, deployment *extensions.Deployment) error
}
