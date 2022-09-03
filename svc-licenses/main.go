//(C) Copyright [2022] Hewlett Packard Enterprise Development LP
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

	"github.com/ODIM-Project/ODIM/lib-utilities/common"
	"github.com/ODIM-Project/ODIM/lib-utilities/config"
	log "github.com/ODIM-Project/ODIM/lib-utilities/logs"
	licenseproto "github.com/ODIM-Project/ODIM/lib-utilities/proto/licenses"
	"github.com/ODIM-Project/ODIM/lib-utilities/services"
	"github.com/ODIM-Project/ODIM/svc-licenses/rpc"
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

	if uid := os.Geteuid(); uid == 0 {
		log.Error("Licenses Service should not be run as the root user")
	}

	if err := config.SetConfiguration(); err != nil {
		log.Error("fatal: error while trying set up configuration: " + err.Error())
	}

	config.CollectCLArgs()

	if err := common.CheckDBConnection(); err != nil {
		log.Error("error while trying to check DB connection health: " + err.Error())
	}
	configFilePath := os.Getenv("CONFIG_FILE_PATH")
	if configFilePath == "" {
		log.Fatal("error: no value get the environment variable CONFIG_FILE_PATH")
	}
	eventChan := make(chan interface{})

	go common.TrackConfigFileChanges(configFilePath, eventChan)

	registerHandlers()

	if err := services.ODIMService.Run(); err != nil {
		log.Error(err)
	}
}

func registerHandlers() {
	if err := services.InitializeService(services.Licenses); err != nil {
		log.Error("fatal: error while trying to initialize service: " + err.Error())
	}
	licenses := rpc.GetLicense()
	licenseproto.RegisterLicensesServer(services.ODIMService.Server(), licenses)
}
