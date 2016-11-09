# Kubernetes Custom Deployment Controller

### Why?

This repo is an example of how you can write a custom controller for Kubernetes using the
[client-go]() package.
This is rather educational code than something you should use as it is.


### How to run?

You can start by creating a Pod:

In Kubernetes:

```
$ kubectl run custom-deployment --image=docker.io/mfojtik/custom-deployment:latest
```

In OpenShift you need to grant the service account rights to access deployments and
replica sets cluster-wide. Be careful as this is horribly unsecure I feel bad for doing
this:

```
$ oadm policy add-cluster-role-to-user admin system:serviceaccount:test:default --as=system:admin
$ kubectl run custom-deployment --image=docker.io/mfojtik/custom-deployment:latest
```

(the project name is 'test' in this case).

Once deployed and the Pod is running, it will observe any new/updated/deleted deployments
and replica sets in the cluster (including itself ;):

```
[@dev] ~ # oc logs custom-deployment-3932074005-ttm95 -f
2016/11/09 16:28:49 Starting custom deployment controller
2016/11/09 16:28:49 Listing all ReplicaSets in cluster
2016/11/09 16:28:49 Listing all Deployment in cluster
2016/11/09 16:28:49 Watching all Deployments in cluster
2016/11/09 16:28:49 Adding deployment custom-deployment
2016/11/09 16:28:49 Watching all ReplicaSets in cluster
2016/11/09 16:28:49 Handling deployment test/custom-deployment
2016/11/09 16:28:49 Finished syncing deployment "test/custom-deployment" (546.3µs)
2016/11/09 16:28:50 Updating deployment custom-deployment
2016/11/09 16:28:50 Handling deployment test/custom-deployment
2016/11/09 16:28:50 Finished syncing deployment "test/custom-deployment" (29.73µs)
```

### How to hack?

Just clone this repository and run `make image`. This should compile the latest binary and
produce `docker.io/mfojtik/custom-deployment:latest` image you can use with commands
described above.

To make sure you're using the local image append `--image-pull-policy=Never` to the `kubectl run`
commands above.
