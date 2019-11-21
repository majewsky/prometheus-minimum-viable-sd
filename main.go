/*******************************************************************************
* Copyright 2019 Stefan Majewsky <majewsky@gmx.net>
* SPDX-License-Identifier: GPL-3.0
* Refer to the file "LICENSE" for details.
*******************************************************************************/

package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const usageText = `
Usage:
	prometheus-minimum-viable-sd announce <path> <address:port>
		Reads a service definition JSON file [1] and announces it to the
		given listener.

	prometheus-minimum-viable-sd collect <path> <address:port>
		Listens on the given TCP address/port for service definitions, and
		collects all service definitions as JSON [1] into the given file.

[1]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config

`

func usage(code int) {
	os.Stderr.Write([]byte(usageText))
	os.Exit(code)
}

func must(err error) {
	if err != nil {
		log.Fatal(err.Error())
	}
}

var debug = false

func main() {
	debug, _ = strconv.ParseBool(os.Getenv("DEBUG"))
	if len(os.Args) != 4 {
		usage(1)
	}

	switch os.Args[1] {
	case "announce":
		announce(os.Args[2], os.Args[3])
	case "collect":
		collect(os.Args[2], os.Args[3])
	case "--help":
		usage(0)
	}
}

type serviceSpec struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

type serviceAnnouncement struct {
	SourceHost string
	ReceivedAt time.Time
	Services   []serviceSpec
}

////////////////////////////////////////////////////////////////////////////////

func announce(inputFile, serverAddress string) {
	announcementBytes, err := ioutil.ReadFile(inputFile)
	must(err)

	//validate early that our input file is a valid service definition
	var announcement []serviceSpec
	err = json.Unmarshal(announcementBytes, &announcement)
	must(err)
	announcementBytes, err = json.Marshal(announcement)
	must(err)

	for {
		sendAnnouncement(announcementBytes, serverAddress)
		time.Sleep(30 * time.Second)
	}
}

func sendAnnouncement(announcement []byte, serverAddress string) {
	conn, err := net.DialTimeout("tcp", serverAddress, 10*time.Second)
	must(err)
	_, err = conn.Write(announcement)
	must(err)
	must(conn.Close())
}

////////////////////////////////////////////////////////////////////////////////

func collect(outputFile, listenAddress string) {
	//The periodic ticks serve two purposes:
	//
	//1. On startup, we don't write into `outputFile` until the first tick. This
	//ensures that clients have enough time to resend their announcements after
	//the listener is restarted. If we wrote `outputFile` immediately after
	//startup, some targets might vanish from Prometheus for a few seconds.
	//
	//2. On each tick, we garbage-collect old announcements that have not been
	//refreshed recently, to account for clients that have shut down.
	canWriteOutputFile := false
	tick := time.Tick(30 * time.Second)

	//ensure that we can write the output file
	must(os.MkdirAll(filepath.Dir(outputFile), 0777))

	//on first startup, initialize the output file immediately so that Prometheus
	//can start watching it
	_, err := os.Stat(outputFile)
	if err != nil && os.IsNotExist(err) {
		must(ioutil.WriteFile(outputFile, []byte("[]"), 0666))
		//since we already wrote into the file, we don't need to wait for the first
		//tick before writing into it again
		canWriteOutputFile = true
	}

	c := make(chan serviceAnnouncement, 10)
	go listenForAnnouncements(listenAddress, c)

	announcements := make(map[string]serviceAnnouncement)
	for {
		select {
		case announcement := <-c:
			if debug {
				log.Println("received announcement from", announcement.SourceHost)
			}
			announcements[announcement.SourceHost] = announcement
			if canWriteOutputFile {
				writeOutputFile(outputFile, announcements)
			}
		case <-tick:
			//see comment at the top of this function
			canWriteOutputFile = true
			minReceivedAt := time.Now().Add(-5 * time.Minute)
			for host, announcement := range announcements {
				if announcement.ReceivedAt.Before(minReceivedAt) {
					delete(announcements, host)
					if debug {
						log.Println("garbage-collecting announcement from", host)
					}
				}
			}
			writeOutputFile(outputFile, announcements)
		}
	}
}

func listenForAnnouncements(listenAddress string, c chan<- serviceAnnouncement) {
	listener, err := net.Listen("tcp", listenAddress)
	must(err)
	for {
		conn, err := listener.Accept()
		must(err)
		go func() {
			remoteHost, _, err := net.SplitHostPort(conn.RemoteAddr().String())
			must(err)
			announcement := serviceAnnouncement{
				SourceHost: remoteHost,
				ReceivedAt: time.Now(),
			}

			must(json.NewDecoder(conn).Decode(&announcement.Services))
			must(conn.Close())
			c <- announcement
		}()
	}
}

func writeOutputFile(outputFile string, announcements map[string]serviceAnnouncement) {
	var allServices []serviceSpec
	for _, announcement := range announcements {
		allServices = append(allServices, announcement.Services...)
	}
	if debug {
		log.Println("writing output file with", len(allServices), "entries")
	}
	servicesBytes, err := json.Marshal(allServices)
	must(err)
	must(ioutil.WriteFile(outputFile, servicesBytes, 0666))
}
