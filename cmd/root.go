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
	"fmt"
	"log"
	"os"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/rest"

	"github.com/mfojtik/custom-deployment/pkg/controllers"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "custom-deployment",
	Short: "Runs a Kubernetes custom deployment strategy controller",
	Long:  ``,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		config, err := rest.InClusterConfig()
		if err != nil {
			log.Fatalf("unable to read in-cluster configuration: %v", err)
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			log.Fatalf("unable to create new configuration for client: %v", err)
		}

		// Check if we can list deployments in cluster
		testd, err := client.Deployments(api.NamespaceAll).List(api.ListOptions{})
		if err != nil {
			log.Fatalf("error: %v", err)
		} else {
			log.Printf("[DEBUG] Can list all %d deployments in cluster", len(testd.Items))
		}

		informers := controllers.NewSharedInformerFactory(client, 10*time.Minute)
		informers.Start(wait.NeverStop)

		c := controllers.NewCustomController(informers.Deployments(), informers.ReplicaSets(), client.Extensions())
		c.Run(5, wait.NeverStop)
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
