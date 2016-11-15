package strategy

import (
	"k8s.io/client-go/1.5/pkg/apis/extensions"
)

type Interface interface {
	Rollout(deployment *extensions.Deployment) error
}
