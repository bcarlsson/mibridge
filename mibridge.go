package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"

	"golang.org/x/net/ipv4"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

// Device struct for json data from Xiaomi gateway
type Device struct {
	Cmd     string `json:"cmd"`
	Token   string `json:"token"`
	Model   string `json:"model"`
	Sid     string `json:"sid"`
	ShortId string `json:"short id"`
	Data    string `json:"data"`
}

// Configuration struct for MQTT server
type Mqtt struct {
	server, user, password, port string
}

// Configuration struct for Multicast interface
type Multicast struct {
	ifName string
}

// Connect to MQTT server
func (m Mqtt) Connect(c chan []string, wg *sync.WaitGroup) {

	//Setting up MQTT client options
	opts := MQTT.NewClientOptions()
	connStr := fmt.Sprintf("tcp://%s:%s", m.server, m.port)
	opts.AddBroker(connStr)
	opts.SetClientID("mibridge")
	client := MQTT.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	log.Println("Connected to MQTT server")

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer client.Disconnect(250)
		var payloadSlice []string

		for {
			payloadSlice = <-c
			log.Println(payloadSlice)
			token := client.Publish(payloadSlice[0], 0, false, payloadSlice[1])
			token.Wait()
		}
	}()
}

// Starting Multicast listener
func (mc Multicast) Start(ch chan []string, wg *sync.WaitGroup) {

	// Verify if supplied interface name is valid
	ifName, err := net.InterfaceByName(mc.ifName)
	if err != nil {
		log.Println("Invalid interface name, trying anyway")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Xiaomi specific multicast settings
		group := net.IPv4(224, 0, 0, 50)
		c, err := net.ListenPacket("udp4", "0.0.0.0:9898")
		if err != nil {
			log.Println(err)
		}
		defer c.Close()

		log.Println("Starting Multicast listener")

		p := ipv4.NewPacketConn(c)
		defer p.Close()
		err = p.JoinGroup(ifName, &net.UDPAddr{IP: group})
		if err != nil {
			log.Println(err)
		}

		log.Println("Joined Multicast group")

		buf := make([]byte, 1024)
		for {
			n, _, _, err := p.ReadFrom(buf)

			parseMsg(string(buf[0:n]), ch)

			if err != nil {
				log.Println("Error: ", err)
			}
		}
	}()
}

func parseMsg(text string, c chan []string) {
	dev := Device{}
	json.Unmarshal([]byte(text), &dev)

	// Unmarshal nested json Data field
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(dev.Data), &data); err != nil {
		panic(err)
	}

	// Checking status of Data field.
	payloadSlice := make([]string, 2, 2)
	if len(data) > 0 {

		// Send payload over channel for publishing
		for i, v := range data {
			payloadSlice[0] = fmt.Sprintf("/mibridge/%s/%s/%s", dev.Model, dev.Sid, i)
			payloadSlice[1] = fmt.Sprintf("%s", v)
			c <- payloadSlice
		}
	} else {
		payloadSlice[0] = fmt.Sprintf("/mibridge/%s/%s", dev.Model, dev.Sid)
		c <- payloadSlice
	}
}

func main() {

	// Connection config for MQTT Server
	mqttServer := Mqtt{
		server:   "192.168.1.120",
		port:     "1883",
		user:     "",
		password: "",
	}

	// Interface name to use for Multicast, ex "eth0"
	multicastServer := Multicast{
		ifName: "wlp1s0",
	}

	c := make(chan []string)

	var wg sync.WaitGroup

	mqttServer.Connect(c, &wg)

	multicastServer.Start(c, &wg)

	wg.Wait()

}
