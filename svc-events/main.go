//(C) Copyright [2020] Hewlett Packard Enterprise Development LP
//
//Licensed under the Apache License, Version 2.0 (the "License"); you may
//not use this file except in compliance with the License. You may obtain
//a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//License for the specific language governing permissions and limitations
// under the License.
package main

import (
	"fmt"
	"os"

	dc "github.com/ODIM-Project/ODIM/lib-messagebus/datacommunicator"
	"github.com/ODIM-Project/ODIM/lib-utilities/common"
	"github.com/ODIM-Project/ODIM/lib-utilities/config"
	log "github.com/ODIM-Project/ODIM/lib-utilities/logs"
	eventsproto "github.com/ODIM-Project/ODIM/lib-utilities/proto/events"
	"github.com/ODIM-Project/ODIM/lib-utilities/services"
	"github.com/ODIM-Project/ODIM/svc-events/consumer"
	"github.com/ODIM-Project/ODIM/svc-events/evcommon"
	"github.com/ODIM-Project/ODIM/svc-events/rpc"
)

func main() {
	// setting up the logging framework
	hostname := os.Getenv("HOST_NAME")
	podname := os.Getenv("POD_NAME")
	pid := os.Getpid()
	c := &log.Config{
		LogFormat: log.SysLogFormat,
		Host:      hostname,
		ProcID:    podname + fmt.Sprintf("_%d", pid),
	}
	log.InitLogger(c)
	// verifying the uid of the user
	if uid := os.Geteuid(); uid == 0 {
		log.Fatal("Event Service should not be run as the root user")
	}

	if err := config.SetConfiguration(); err != nil {
		log.Fatal("fatal: error while trying set up configuration: " + err.Error())
	}

	config.CollectCLArgs()

	if err := dc.SetConfiguration(config.Data.MessageBusConf.MessageBusConfigFilePath); err != nil {
		log.Fatal("error while trying to set messagebus configuration: " + err.Error())
	}
	if err := common.CheckDBConnection(); err != nil {
		log.Fatal("error while trying to check DB connection health: " + err.Error())
	}

	if err := services.InitializeService(services.Events); err != nil {
		log.Fatal("fatal: error while trying to initialize the service: " + err.Error())
	}

	// Intializing the TopicsList
	evcommon.EMBTopics.TopicsList = make(map[string]bool)
	// Intializing plugin token
	evcommon.Token.Tokens = make(map[string]string)

	// register handlers
	events := rpc.GetPluginContactInitializer()
	eventsproto.RegisterEventsServer(services.ODIMService.Server(), events)

	// CreateJobQueue defines the queue which will act as an infinite buffer
	// In channel is an entry or input channel and the Out channel is an exit or output channel
	jobQueueSize := 10
	consumer.In, consumer.Out = common.CreateJobQueue(jobQueueSize)
	// RunReadWorkers will create a worker pool for doing a specific task
	// which is passed to it as PublishEventsToDestination method after reading the data from the channel.
	common.RunReadWorkers(consumer.Out, events.Connector.PublishEventsToDestination, 5)

	// CreateJobQueue defines the queue which will act as an infinite buffer
	// In channel is an entry or input channel and the Out channel is an exit or output channel
	ctrlMsgProcQueueSize := 1
	consumer.CtrlMsgRecvQueue, consumer.CtrlMsgProcQueue = common.CreateJobQueue(ctrlMsgProcQueueSize)
	// RunReadWorkers will create a worker pool for doing a specific task
	// which is passed to it as ProcessCtrlMsg method after reading the data from the channel.
	common.RunReadWorkers(consumer.CtrlMsgProcQueue, evcommon.ProcessCtrlMsg, 1)

	configFilePath := os.Getenv("CONFIG_FILE_PATH")
	if configFilePath == "" {
		log.Fatal("error: no value get the environment variable CONFIG_FILE_PATH")
	}
	eventChan := make(chan interface{})
	// TrackConfigFileChanges monitors the odim config changes using fsnotfiy
	go common.TrackConfigFileChanges(configFilePath, eventChan)

	// Subscribe to intercomm messagebus queue
	go consumer.SubscribeCtrlMsgQueue(config.Data.MessageBusConf.MessageBusQueue[0])

	// Subscribe to EMBs of all the available plugins
	startUPInterface := evcommon.StartUpInteraface{
		DecryptPassword: common.DecryptWithPrivateKey,
		EMBConsume:      consumer.Consume,
	}
	go startUPInterface.SubscribePluginEMB()

	// Run server
	if err := services.ODIMService.Run(); err != nil {
		log.Fatal(err.Error())
	}
}
