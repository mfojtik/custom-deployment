package framework

import "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"

type Deployer interface {
	Sync(*v1beta1.Deployment) error
	SyncStatus(*v1beta1.Deployment) error
	HandleOverlap(*v1beta1.Deployment) error
	HandlePaused(*v1beta1.Deployment) error
	HandleFailed(*v1beta1.Deployment) error
	HandleScaling(*v1beta1.Deployment) error
	Rollback(*v1beta1.Deployment, int32) error
	RunStrategy(*v1beta1.Deployment) error
}
