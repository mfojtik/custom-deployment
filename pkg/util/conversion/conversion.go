package conversion

import (
	"log"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

func PodToInternal(in *v1.Pod) *api.Pod {
	out := &api.Pod{}
	if err := v1.Convert_v1_Pod_To_api_Pod(in, out, nil); err != nil {
		log.Printf("failed to convert pod %#+v to internal: %v", in, err)
		return nil
	}
	return out
}

func PodToVersioned(in *api.Pod) *v1.Pod {
	out := &v1.Pod{}
	if err := v1.Convert_api_Pod_To_v1_Pod(in, out, nil); err != nil {
		log.Printf("failed to convert pod %#+v to external: %v", in, err)
		return nil
	}
	return out
}

func ReplicaSetToInternal(in *v1beta1.ReplicaSet) *extensions.ReplicaSet {
	out := &extensions.ReplicaSet{}
	if err := v1beta1.Convert_v1beta1_ReplicaSet_To_extensions_ReplicaSet(in, out, nil); err != nil {
		log.Printf("failed to convert replica set %#+v to internal: %v", in, err)
		return nil
	}
	return out
}

func ReplicaSetToExternal(in *extensions.ReplicaSet) *v1beta1.ReplicaSet {
	out := &v1beta1.ReplicaSet{}
	if err := v1beta1.Convert_extensions_ReplicaSet_To_v1beta1_ReplicaSet(in, out, nil); err != nil {
		log.Printf("failed to convert replica set %#+v to external: %v", in, err)
		return nil
	}
	return out
}

func DeploymentToInternal(in *v1beta1.Deployment) *extensions.Deployment {
	out := &extensions.Deployment{}
	if err := v1beta1.Convert_v1beta1_Deployment_To_extensions_Deployment(in, out, nil); err != nil {
		log.Printf("failed to convert deployment %#+v to internal: %v", in, err)
		return nil
	}
	return out
}

func DeploymentToExternal(in *extensions.Deployment) *v1beta1.Deployment {
	out := &v1beta1.Deployment{}
	if err := v1beta1.Convert_extensions_Deployment_To_v1beta1_Deployment(in, out, nil); err != nil {
		log.Printf("failed to convert deployment %#+v to external: %v", in, err)
		return nil
	}
	return out
}
