package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/naoina/toml"
	"github.com/withmandala/go-log"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	influxclient "github.com/influxdata/influxdb1-client/v2"
)

// MQTT settings for overall configuration
type tomlConfigMQTT struct {
	BrokerHost     string
	BrokerPort     int
	BrokerUsername string
	BrokerPassword string
	ClientId       string
	TopicPrefix    string
	Topic          string
}

type tomlConfigHass struct {
	Discovery       bool
	DiscoveryPrefix string
	ObjectId        string
	DeviceModel     string
	DeviceName      string
	Manufacturer    string
}

type tomlConfigInflux struct {
	Hostname string
	Port     int
	Database string
	Username string
	Password string
}

type tomlConfigPurpleAir struct {
	Url      string
	PollRate int
}

type tomlConfig struct {
	PurpleAir tomlConfigPurpleAir
	Mqtt      tomlConfigMQTT
	Hass      tomlConfigHass
	Influx    tomlConfigInflux
}

type purpleAirMonitor struct {
	SensorId         string  `json:"SensorId"`
	DateTime         string  `json:"DateTime"`
	Sensor           string  `json:"Sensor"`
	PM25AqiColor     string  `json:"p25aqic"`
	PM25Aqi          int     `json:"pm2.5_aqi"`
	PM10Cf1          float32 `json:"pm1_0_cf_1"`
	P03um            float32 `json:"p_0_3_um"`
	PM25Cf1          float32 `json:"pm2_5_cf_1"`
	P05um            float32 `json:"p_0_5_um"`
	PM100Cf1         float32 `json:"pm10_0_cf_1"`
	P10um            float32 `json:"p_1_0_um"`
	PM10Atm          float32 `json:"pm1_0_atm"`
	P25um            float32 `json:"p_2_5_um"`
	PM25Atm          float32 `json:"pm2_5_atm"`
	P50um            float32 `json:"p_5_0_um"`
	PM100Atm         float32 `json:"pm10_0_atm"`
	P100um           float32 `json:"p_10_0_um"`
	Key1Response     int     `json:"key1_response"`
	Key1ResponseDate int     `json:"key1_response_date"`
	Key1Count        int     `json:"key1_count"`
	TsLatency        int     `json:"ts_latency"`
	Key2Response     int     `json:"key2_response"`
	Key2ResponseDate int     `json:"key2_response_date"`
	Key2Count        int     `json:"key2_count"`
	TsSLatency       int     `json:"ts_s_latency"`
}

type purpleAirStatus struct {
	SensorId           string  `json:"SensorId"`           // MAC address of the device
	DateTime           string  `json:"DateTime"`           // UTC datetime on the device
	Geo                string  `json:"Geo"`                // name of the device
	Memory             int     `json:"Mem"`                // free memory on the device?
	MemFrag            int     `json:"memfrag"`            // ???
	MemFB              int     `json:"memfb"`              // ???
	MemCS              int     `json:"memcs"`              // ???
	Id                 int     `json:"Id"`                 // ???
	Latitude           float32 `json:"lat"`                // configured latitude
	Longitude          float32 `json:"long"`               // configure longitude
	ADC                float32 `json:"Adc"`                // ???
	LoggingRate        int     `json:"loggingrate"`        // how often the data are updated
	Place              string  `json:"place"`              // location of the device
	Version            string  `json:"version"`            // firmware version of the device
	Uptime             int     `json:"uptime"`             // number of seconds since last reboot
	RSSI               int     `json:"rssi"`               // wifi signal strength
	Period             int     `json:"period"`             // number of seconds for averaging?
	HttpSuccess        int     `json:"httpsuccess"`        // number of successful HTTP requests
	HttpSends          int     `json:"httpsends"`          // total number of http sends
	HardwareRevision   string  `json:"hardwarerevision"`   // version number of the physical hardware
	HardwareDiscovered string  `json:"hardwarediscovered"` // list of the hardware present on the device
	Temperature        int     `json:"current_temp_f"`     // current fahrenheit temperature rounded to nearest degree
	Humidity           int     `json:"current_humidity"`   // current humidity rounded to nearest percent
	Dewpoint           int     `json:"current_dewpoint_f"` // current dewpoint in fahrenheit rounded to nearest degree
	Pressure           float32 `json:"pressure"`           // current pressure in mmHg

	A                purpleAirMonitor `json:sensor_a,omitempty` // breakout for sensor a
	PM25AqiColor     string           `json:"p25aqic"`
	PM25Aqi          int              `json:"pm2.5_aqi"`
	PM10Cf1          float32          `json:"pm1_0_cf_1"`
	P03um            float32          `json:"p_0_3_um"`
	PM25Cf1          float32          `json:"pm2_5_cf_1"`
	P05um            float32          `json:"p_0_5_um"`
	PM100Cf1         float32          `json:"pm10_0_cf_1"`
	P10um            float32          `json:"p_1_0_um"`
	PM10Atm          float32          `json:"pm1_0_atm"`
	P25um            float32          `json:"p_2_5_um"`
	PM25Atm          float32          `json:"pm2_5_atm"`
	P50um            float32          `json:"p_5_0_um"`
	PM100Atm         float32          `json:"pm10_0_atm"`
	P100um           float32          `json:"p_10_0_um"`
	Key1Response     int              `json:"key1_response"`
	Key1ResponseDate int              `json:"key1_response_date"`
	Key1Count        int              `json:"key1_count"`
	TsLatency        int              `json:"ts_latency"`
	Key2Response     int              `json:"key2_response"`
	Key2ResponseDate int              `json:"key2_response_date"`
	Key2Count        int              `json:"key2_count"`
	TsSLatency       int              `json:"ts_s_latency"`

	B                 purpleAirMonitor `json:sensor_b,omitempty` // breakout for sensor b
	PM25AqiColorB     string           `json:"p25aqic_b"`
	PM25AqiB          int              `json:"pm2.5_aqi_b"`
	PM10Cf1B          float32          `json:"pm1_0_cf_1_b"`
	P03umB            float32          `json:"p_0_3_um_b"`
	PM25Cf1B          float32          `json:"pm2_5_cf_1_b"`
	P05umB            float32          `json:"p_0_5_um_b"`
	PM100Cf1B         float32          `json:"pm10_0_cf_1_b"`
	P10umB            float32          `json:"p_1_0_um_b"`
	PM10AtmB          float32          `json:"pm1_0_atm_b"`
	P25umB            float32          `json:"p_2_5_um_b"`
	PM25AtmB          float32          `json:"pm2_5_atm_b"`
	P50umB            float32          `json:"p_5_0_um_b"`
	PM100AtmB         float32          `json:"pm10_0_atm_b"`
	P100umB           float32          `json:"p_10_0_um_b"`
	Key1ResponseB     int              `json:"key1_response_b"`
	Key1ResponseDateB int              `json:"key1_response_date_b"`
	Key1CountB        int              `json:"key1_count_b"`
	TsLatencyB        int              `json:"ts_latency_b"`
	Key2ResponseB     int              `json:"key2_response_b"`
	Key2ResponseDateB int              `json:"key2_response_date_b"`
	Key2CountB        int              `json:"key2_count_b"`
	TsSLatencyB       int              `json:"ts_s_latency_b"`

	PaLatency     int    `json:"pa_latency"`
	Response      int    `json:"response"`
	ResponseDate  int    `json:"response_date"`
	Latency       int    `json:"latency"`
	WirelessState string `json:"wlstate"`
	Status0       int    `json:"status_0"`
	Status1       int    `json:"status_1"`
	Status2       int    `json:"status_2"`
	Status3       int    `json:"status_3"`
	Status4       int    `json:"status_4"`
	Status5       int    `json:"status_5"`
	Status6       int    `json:"status_6"`
	Status7       int    `json:"status_7"`
	Status8       int    `json:"status_8"`
	Status9       int    `json:"status_9"`
	SSID          string `json:"ssid"`
}

// set up a global logger...
// see: https://stackoverflow.com/a/43827612/57626
var logger *log.Logger

var config tomlConfig

// var components tomlComponents
var client mqtt.Client

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	r := client.OptionsReader()
	logger.Infof("connected to MQTT at %s", r.Servers())
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	logger.Errorf("Connect lost: %v", err)
}

func main() {
	logger = log.New(os.Stderr).WithColor()

	configFile := flag.String("config", "", "Filename with configuration")
	flag.Parse()

	if *configFile != "" {
		f, err := os.Open(*configFile)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := toml.NewDecoder(f).Decode(&config); err != nil {
			panic(err)
		}
	} else {
		logger.Fatal("Must specify configuration file with -config FILENAME")
	}

	opts := mqtt.NewClientOptions()

	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Mqtt.BrokerHost, config.Mqtt.BrokerPort))
	if config.Mqtt.BrokerPassword != "" && config.Mqtt.BrokerUsername != "" {
		opts.SetUsername(config.Mqtt.BrokerUsername)
		opts.SetPassword(config.Mqtt.BrokerPassword)
	}
	opts.SetClientID(config.Mqtt.ClientId)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	logger.Infof("HTTP Target: %s", config.PurpleAir.Url)

	var myClient = &http.Client{Timeout: 10 * time.Second}

	for {
		pastatus := new(purpleAirStatus)
		// see: https://stackoverflow.com/a/31129967/57626
		getJson(config.PurpleAir.Url, pastatus, myClient)
		normalizePaStatus(pastatus)

		logger.Infof("Sensor ID: %s", pastatus.SensorId)
		logger.Infof("Timestamp: %s", pastatus.DateTime)
		logger.Infof("Color: %s", pastatus.PM25AqiColor)
		logger.Infof("AQI: %d", pastatus.PM25Aqi)
		logger.Infof("Color: %s", pastatus.PM25AqiColorB)
		logger.Infof("AQI: %d", pastatus.B.PM25Aqi)
		logger.Debugf("Sleeping for %d seconds", config.PurpleAir.PollRate)
		write_influx(pastatus, &pastatus.A, &pastatus.B)
		time.Sleep(time.Duration(config.PurpleAir.PollRate) * time.Second)
	}
}

func normalizePaStatus(pastatus *purpleAirStatus) *purpleAirStatus {
	pastatus.A.SensorId = pastatus.SensorId
	pastatus.A.DateTime = pastatus.DateTime
	pastatus.A.Sensor = "A"
	pastatus.A.PM25AqiColor = pastatus.PM25AqiColor
	pastatus.A.PM25Aqi = pastatus.PM25Aqi
	pastatus.A.PM10Cf1 = pastatus.PM10Cf1
	pastatus.A.P03um = pastatus.P03um
	pastatus.A.PM25Cf1 = pastatus.PM25Cf1
	pastatus.A.P05um = pastatus.P05um
	pastatus.A.PM100Cf1 = pastatus.PM100Cf1
	pastatus.A.P10um = pastatus.P10um
	pastatus.A.PM10Atm = pastatus.PM10Atm
	pastatus.A.P25um = pastatus.P25um
	pastatus.A.PM25Atm = pastatus.PM25Atm
	pastatus.A.P50um = pastatus.P50um
	pastatus.A.PM100Atm = pastatus.PM100Atm
	pastatus.A.P100um = pastatus.P100um
	pastatus.A.Key1Response = pastatus.Key1Response
	pastatus.A.Key1ResponseDate = pastatus.Key1ResponseDate
	pastatus.A.Key1Count = pastatus.Key1Count
	pastatus.A.TsLatency = pastatus.TsLatency
	pastatus.A.Key2Response = pastatus.Key2Response
	pastatus.A.Key2ResponseDate = pastatus.Key2ResponseDate
	pastatus.A.Key2Count = pastatus.Key2Count
	pastatus.A.TsSLatency = pastatus.TsSLatency

	pastatus.B.SensorId = pastatus.SensorId
	pastatus.B.DateTime = pastatus.DateTime
	pastatus.B.Sensor = "B"
	pastatus.B.PM25AqiColor = pastatus.PM25AqiColorB
	pastatus.B.PM25Aqi = pastatus.PM25AqiB
	pastatus.B.PM10Cf1 = pastatus.PM10Cf1B
	pastatus.B.P03um = pastatus.P03umB
	pastatus.B.PM25Cf1 = pastatus.PM25Cf1B
	pastatus.B.P05um = pastatus.P05umB
	pastatus.B.PM100Cf1 = pastatus.PM100Cf1B
	pastatus.B.P10um = pastatus.P10umB
	pastatus.B.PM10Atm = pastatus.PM10AtmB
	pastatus.B.P25um = pastatus.P25umB
	pastatus.B.PM25Atm = pastatus.PM25AtmB
	pastatus.B.P50um = pastatus.P50umB
	pastatus.B.PM100Atm = pastatus.PM100AtmB
	pastatus.B.P100um = pastatus.P100umB
	pastatus.B.Key1Response = pastatus.Key1ResponseB
	pastatus.B.Key1ResponseDate = pastatus.Key1ResponseDateB
	pastatus.B.Key1Count = pastatus.Key1CountB
	pastatus.B.TsLatency = pastatus.TsLatencyB
	pastatus.B.Key2Response = pastatus.Key2ResponseB
	pastatus.B.Key2ResponseDate = pastatus.Key2ResponseDateB
	pastatus.B.Key2Count = pastatus.Key2CountB
	pastatus.B.TsSLatency = pastatus.TsSLatencyB
	return pastatus
}

func getJson(url string, target interface{}, myClient *http.Client) error {
	r, err := myClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func status_to_point(status *purpleAirStatus) (*influxclient.Point, error) {
	tags := map[string]string{"sensorId": status.SensorId}
	values := map[string]interface{}{}

	values["temperature"] = status.Temperature
	values["humidity"] = status.Humidity
	values["pressure"] = status.Pressure
	values["dewpoint"] = status.Dewpoint
	values["rssi"] = status.RSSI

	return influxclient.NewPoint("system", tags, values, time.Now())
}

func monitor_to_point(monitor *purpleAirMonitor) (*influxclient.Point, error) {
	tags := map[string]string{"sensorId": monitor.SensorId, "sensor": monitor.Sensor}
	values := map[string]interface{}{}
	values["pm2.5_aqic"] = monitor.PM25AqiColor
	values["pm2.5_aqi"] = monitor.PM25Aqi
	values["pm1.0_cf_1"] = monitor.PM10Cf1
	values["pm0.3_um"] = monitor.P03um
	values["pm2.5_cf_1"] = monitor.PM25Cf1
	values["pm0.5_um"] = monitor.P05um
	values["pm10.0_cf_1"] = monitor.PM100Cf1
	values["pm1.0_um"] = monitor.P10um
	values["pm1.0_atm"] = monitor.PM10Atm
	values["pm2.5_um"] = monitor.P25um
	values["pm2.5_atm"] = monitor.PM25Atm
	values["pm5.0_um"] = monitor.P50um
	values["pm10.0_atm"] = monitor.PM100Atm
	values["pm10.0_um"] = monitor.P100um
	values["key1_response"] = monitor.Key1Response
	values["key1_response_date"] = monitor.Key1ResponseDate
	values["key1_count"] = monitor.Key1Count
	values["ts_latency"] = monitor.TsLatency
	values["key2_response"] = monitor.Key2Response
	values["key2_response_date"] = monitor.Key2ResponseDate
	values["key2_count"] = monitor.Key2Count
	values["ts_s_latency"] = monitor.TsSLatency

	return influxclient.NewPoint("purpleair", tags, values, time.Now())
}

func write_influx(status *purpleAirStatus, monitorA *purpleAirMonitor, monitorB *purpleAirMonitor) {
	c, err := influxclient.NewHTTPClient(influxclient.HTTPConfig{
		Addr: fmt.Sprintf("http://%s:%d", config.Influx.Hostname, config.Influx.Port),
	})
	if err != nil {
		logger.Errorf("Error creating InfluxDB Client: ", err.Error())
	}
	defer c.Close()

	bp, err := influxclient.NewBatchPoints(influxclient.BatchPointsConfig{
		Database:  config.Influx.Database,
		Precision: "s",
	})
	if err != nil {
		logger.Errorf("error creating batchpoints: %s", err)
	}

	pointA, err := monitor_to_point(monitorA)
	if err != nil {
		logger.Errorf("error translating monitor sample to point")
	}
	bp.AddPoint(pointA)

	pointB, err := monitor_to_point(monitorB)
	if err != nil {
		logger.Errorf("error translating monitor sample to point")
	}
	bp.AddPoint(pointB)

	pointS, err := status_to_point(status)
	if err != nil {
		logger.Errorf("error translating status to point")
	}
	bp.AddPoint(pointS)

	err = c.Write(bp)

	if err != nil {
		logger.Fatal(err)
	}
}
