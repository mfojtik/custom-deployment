package conversion

import (
	"log"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

func PodToInternal(in *v1.Pod) *api.Pod {
	out := api.Pod{}
	if err := api.Scheme.Convert(in, &out, nil); err != nil {
		log.Printf("failed to convert pod %#+v to internal: %v", in, err)
		return nil
	}
	return &out
}

func PodToVersioned(in *api.Pod) *v1.Pod {
	out := v1.Pod{}
	if err := api.Scheme.Convert(in, &out, nil); err != nil {
		log.Printf("failed to convert pod %#+v to external: %v", in, err)
		return nil
	}
	return &out
}

func ReplicaSetToInternal(in *v1beta1.ReplicaSet) *extensions.ReplicaSet {
	out := extensions.ReplicaSet{}
	if err := api.Scheme.Convert(in, &out, nil); err != nil {
		log.Printf("failed to convert replica set %#+v to internal: %v", in, err)
		return nil
	}
	return &out
}

func ReplicaSetToExternal(in *extensions.ReplicaSet) *v1beta1.ReplicaSet {
	out := v1beta1.ReplicaSet{}
	if err := api.Scheme.Convert(in, &out, nil); err != nil {
		log.Printf("failed to convert replica set %#+v to external: %v", in, err)
		return nil
	}
	return &out
}

func DeploymentToInternal(in *v1beta1.Deployment) *extensions.Deployment {
	var out extensions.Deployment
	if err := api.Scheme.Convert(in, &out, nil); err != nil {
		log.Printf("failed to convert deployment %#+v to internal: %v", in, err)
		return nil
	}
	return &out
}

func DeploymentToExternal(in *extensions.Deployment) *v1beta1.Deployment {
	out := v1beta1.Deployment{}
	if err := api.Scheme.Convert(in, &out, nil); err != nil {
		log.Printf("failed to convert deployment %#+v to external: %v", in, err)
		return nil
	}
	return &out
}
