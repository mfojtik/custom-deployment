package deployments

import "k8s.io/client-go/1.5/pkg/apis/extensions"

// FilterActiveReplicaSets returns replica sets that have (or at least ought to have) pods.
func FilterActiveReplicaSets(replicaSets []*extensions.ReplicaSet) []*extensions.ReplicaSet {
	active := []*extensions.ReplicaSet{}
	for i := range replicaSets {
		rs := replicaSets[i]

		if rs != nil && rs.Spec.Replicas > 0 {
			active = append(active, replicaSets[i])
		}
	}
	return active
}
