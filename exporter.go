// Reads CO2 concentration and temperature from an MH-Z19 sensor, publishing prometheus metrics over HTTP.
package main

import (
	"flag"
	"io"
	"net/http"
	"sync"
	"text/template"

	"log"

	"github.com/mhansen/mhz19"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jacobsa/go-serial/serial"
)

const prefix = "mhz19"

var (
	portname = flag.String("portname", "", "filename of serial port")
	port     = flag.String("port", ":8080", "http port to listen on")
	index    = template.Must(template.New("index").Parse(
		`<!doctype html>
	 <title>MH-Z19 Carbon Dioxide Sensor Prometheus Exporter</title>
	 <h1>MH-Z19 Carbon Dioxide Sensor Prometheus Exporter</h1>
	 <a href="/metrics">Metrics</a>
	 <p>
	 <pre>portname={{.}}</pre>
	 `))
)

func main() {
	flag.Parse()
	log.Printf("MH-Z19 Carbon Dioxide Sensor Prometheus Exporter starting on port %v and file %v\n", *port, *portname)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		index.Execute(w, *portname)
	})

	options := serial.OpenOptions{
		PortName:              *portname,
		BaudRate:              9600,
		DataBits:              8,
		StopBits:              1,
		InterCharacterTimeout: 1000,
	}

	serialPort, err := serial.Open(options)
	if err != nil {
		log.Fatalf("serial.Open %v failed: %v", *portname, err)
	}
	defer serialPort.Close()

	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		prometheus.NewGoCollector(),
		prometheus.NewBuildInfoCollector(),
		&mhz19Collector{serialPort: serialPort},
	)
	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	http.ListenAndServe(*port, nil)
}

type mhz19Collector struct {
	mu         sync.Mutex // serial port is shared resource and this runs in HTTP handler goroutines
	serialPort io.ReadWriter
}

func (c *mhz19Collector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(c, ch)
}

func (c *mhz19Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	err := mhz19.NewGasConcentrationRequest().Write(c.serialPort)
	if err != nil {
		log.Fatalf("couldn't write to serial port: %v", err)
	}

	resp, err := mhz19.ReadGasConcentrationResponse(c.serialPort)
	if err != nil {
		if _, ok := err.(*mhz19.ChecksumError); ok {
			log.Printf("checksum error: %v", err)
			return
		}
		log.Printf("readGasConcentration error: %v", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			prefix+"_co2_concentration_ppm",
			"Carbon Dioxide Concentration in parts per million",
			[]string{},
			nil),
		prometheus.GaugeValue,
		float64(resp.Concentration),
	)
	ch <- prometheus.MustNewConstMetric(
		prometheus.NewDesc(
			prefix+"_temperature_celsius",
			"Sensor Temperature in degrees Celsius",
			[]string{},
			nil),
		prometheus.GaugeValue,
		float64(resp.Temperature()),
	)
}
