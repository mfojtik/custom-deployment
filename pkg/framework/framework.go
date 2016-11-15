package framework

import (
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/kubernetes/pkg/apis/extensions/v1beta1"

	"github.com/mfojtik/custom-deployment/pkg/controllers"
	"github.com/mfojtik/custom-deployment/pkg/informers"
)

type CustomDeployer struct {
}

func (c *CustomDeployer) Run(client *kubernetes.Clientset, defaultResync int, numWorkers int) {
	stopChan := make(chan struct{})
	sharedInformer := informers.NewSharedInformerFactory(client, startOptions.Namespace, time.Duration(defaultResync)*time.Minute)
	controller := controllers.NewCustomController(sharedInformer.Deployments(), sharedInformer.ReplicaSets(), client.Extensions(), c)
	go func() {
		controller.Run(numWorkers, stopChan)
	}()
	sharedInformer.Start(stopChan)
	<-stopChan
}

func (c *CustomDeployer) Sync(d *v1beta1.Deployment) error          {}
func (c *CustomDeployer) SyncStatus(d *v1beta1.Deployment) error    {}
func (c *CustomDeployer) HandleOverlap(d *v1beta1.Deployment) error {}
func (c *CustomDeployer) HandlePaused(d *v1beta1.Deployment) error  {}
func (c *CustomDeployer) HandleFailed(d *v1beta1.Deployment) error  {}
func (c *CustomDeployer) HandleScaling(d *v1beta1.Deployment) error {}
func (c *CustomDeployer) Rollback(d *v1beta1.Deployment) error      {}
func (c *CustomDeployer) RunStrategy(d *v1beta1.Deployment) error   {}
