package controllers

import (
	"fmt"
	"log"
	"time"

	"github.com/mfojtik/custom-deployment/pkg/util/workqueue"
	typed "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

type CustomController struct {
	extensionsClient typed.ExtensionsInterface
	deploymentLister *cache.StoreToDeploymentLister
	replicaSetLister *cache.StoreToReplicaSetLister

	deploymentSynced cache.InformerSynced
	replicaSetSynced cache.InformerSynced

	queue workqueue.RateLimitingInterface
}

func NewCustomController(dInformer DeploymentInformer, rsInformer ReplicaSetInformer, extClient typed.ExtensionsInterface) *CustomController {
	c := &CustomController{
		extensionsClient: extClient,
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "customdeployment"),
	}

	dInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeploymentNotification,
		UpdateFunc: c.updateDeploymentNotification,
		DeleteFunc: c.deleteDeploymentNotification,
	})

	rsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})

	c.deploymentLister = dInformer.Lister()
	c.replicaSetLister = rsInformer.Lister()

	c.deploymentSynced = dInformer.Informer().HasSynced
	c.replicaSetSynced = rsInformer.Informer().HasSynced

	return c
}

func (c *CustomController) addDeploymentNotification(obj interface{}) {
	d := obj.(*v1beta1.Deployment)
	log.Printf("Adding deployment %s", d.Name)
	c.enqueueDeployment(d)
}

func (c *CustomController) updateDeploymentNotification(oldObj, newObj interface{}) {
	oldD := oldObj.(*v1beta1.Deployment)
	log.Printf("Updating deployment %s", oldD.Name)
	c.enqueueDeployment(newObj.(*v1beta1.Deployment))
}

func (c *CustomController) deleteDeploymentNotification(obj interface{}) {
	d, ok := obj.(*v1beta1.Deployment)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Printf("Couldn't get object from tombstone %#v", obj)
			return
		}
		d, ok = tombstone.Obj.(*v1beta1.Deployment)
		if !ok {
			log.Printf("Tombstone contained object that is not a Deployment %#v", obj)
			return
		}
	}
	log.Printf("Deleting deployment %s", d.Name)
	c.enqueueDeployment(d)
}

func (c *CustomController) enqueueDeployment(deployment *v1beta1.Deployment) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(deployment)
	if err != nil {
		log.Printf("Couldn't get key for object %#v: %v", deployment, err)
		return
	}

	c.queue.Add(key)
}

func (c *CustomController) Run(threadiness int, stopCh <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.ShutDown()

	log.Printf("Starting custom deployment controller")

	// wait for your secondary caches to fill before starting your work
	if !cache.WaitForCacheSync(stopCh, c.deploymentSynced) || !cache.WaitForCacheSync(stopCh, c.replicaSetSynced) {
		return
	}

	log.Printf("Caches has been synced, starting workers")

	for i := 0; i < threadiness; i++ {
		log.Printf("Starting worker %d", i)
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	// wait until we're told to stop
	<-stopCh
	log.Printf("Shutting down custom deployment controller")
}

func (c *CustomController) runWorker() {
	for {
		if quit := c.processNextWorkItem(); quit {
			break
		}
	}
}

// processNextWorkItem deals with one key off the queue.  It returns false when it's time to quit.
func (c *CustomController) processNextWorkItem() bool {
	work := func() bool {
		key, quit := c.queue.Get()
		if quit {
			return true
		}
		defer c.queue.Done(key)

		if err := c.syncDeployment(key.(string)); err != nil {
			log.Printf("error syncing %v: %v", key, err)
			utilruntime.HandleError(fmt.Errorf("%v failed with : %v", key, err))
			c.queue.AddRateLimited(key)
			return false
		}

		return false
	}

	for {
		if quit := work(); quit {
			return true
		}
	}
}

func (c *CustomController) syncDeployment(key string) error {
	startTime := time.Now()
	defer func() {
		log.Printf("Finished syncing deployment %q (%v)", key, time.Now().Sub(startTime))
	}()

	obj, exists, err := c.deploymentLister.Indexer.GetByKey(key)
	if err != nil {
		log.Printf("Unable to retrieve deployment %v from store: %v", key, err)
		return err
	}
	if !exists {
		log.Printf("Deployment has been deleted %v", key)
		return nil
	}

	deployment := obj.(*v1beta1.Deployment)
	d, err := deploymentDeepCopy(deployment)
	if err != nil {
		return err
	}

	log.Printf("Handling deployment %s/%s", d.Namespace, d.Name)

	return nil
}

func deploymentDeepCopy(deployment *v1beta1.Deployment) (*v1beta1.Deployment, error) {
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
