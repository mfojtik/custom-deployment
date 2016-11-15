// Copyright © 2016 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"flag"
	"log"
	"time"

	"github.com/mfojtik/custom-deployment/pkg/controllers"
	"github.com/mfojtik/custom-deployment/pkg/informers"
	"github.com/mfojtik/custom-deployment/pkg/strategy/custom"
	"github.com/spf13/cobra"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/rest"
)

type StartOptions struct {
	Namespace     string
	DefaultResync int
	Workers       int
	LogLevel      string
}

var startOptions = &StartOptions{}

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Starts a custom deployment controller",
	Long: `This commands starts a new deployment controller.

This must be run inside a Pod and have service account mounted that allows to
manage Deployments and ReplicaSets.
`,
	Run: func(cmd *cobra.Command, args []string) {
		flag.Set("loglevel", startOptions.LogLevel)
		config, err := rest.InClusterConfig()
		if err != nil {
			log.Fatalf("reading cluster configuration failed: %v", err)
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			log.Fatalf("unable to construct kubernetes client: %v", err)
		}

		// TODO: Add signal handler for SIGINT
		stopChan := make(chan struct{})
		sharedInformer := informers.NewSharedInformerFactory(client, startOptions.Namespace, time.Duration(startOptions.DefaultResync)*time.Minute)
		strategy := custom.NewStrategy(sharedInformer.ReplicaSets(), client.Extensions(), client.Core())

		c := controllers.NewCustomController(sharedInformer.Deployments(), client.Extensions(), strategy)
		go func() {
			c.Run(startOptions.Workers, stopChan)
		}()
		sharedInformer.Start(stopChan)
		<-stopChan
	},
}

func init() {
	RootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&startOptions.Namespace, "namespace", "n", "", "Namespace to handle by this controller (default: all namespaces)")
	startCmd.Flags().IntVarP(&startOptions.DefaultResync, "default-resync-minutes", "", 10, "Default resync interval")
	startCmd.Flags().IntVarP(&startOptions.Workers, "workers", "", 5, "Default numbers of workers to start")
	startCmd.Flags().StringVarP(&startOptions.LogLevel, "loglevel", "v", "5", "Default verbosity level")
}
