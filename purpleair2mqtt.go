package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/naoina/toml"
	"github.com/withmandala/go-log"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	influxclient "github.com/influxdata/influxdb1-client/v2"
)

// Types for Home Assistant MQTT Discovery
type hassMqttConfigDevice struct {
	Identifiers  []string `json:"identifiers"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	Name         string   `json:"name"`
	SWVersion    string   `json:"sw_version"`
}

type hassMqttConfig struct {
	AvailabilityTopic string               `json:"availability_topic"`
	ConfigTopic       string               `json:"-"`
	Device            hassMqttConfigDevice `json:"device"`
	DeviceClass       string               `json:"device_class,omitempty"`
	Name              string               `json:"name"`
	Qos               int                  `json:"qos"`
	StateTopic        string               `json:"state_topic"`
	UniqueId          string               `json:"unique_id"`
	Icon              string               `json:"icon,omitempty"`
	UnitOfMeasurement string               `json:"unit_of_measurement"`
	Platform          string               `json:"-"`
}

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
	Timescale tomlConfigTimescale
}

type purpleAirMonitor struct {
	SensorId         string  `json:"SensorId" pg:"sensor_id"`
	DateTime         string  `json:"DateTime"`
	Sensor           string  `json:"Sensor" pg:"channel"`
	PM25AqiColor     string  `json:"p25aqic"`
	PM25Aqi          int     `json:"pm2.5_aqi" pg:"pm2_5_aqi"`
	PM10Cf1          float64 `json:"pm1_0_cf_1" pg:"pm1_0_cf_1"`
	P03um            float64 `json:"p_0_3_um" pg:"p_0_3_um"`
	PM25Cf1          float64 `json:"pm2_5_cf_1" pg:"pm2_5_cf_1"`
	P05um            float64 `json:"p_0_5_um" pg:"p_0_5_um"`
	PM100Cf1         float64 `json:"pm10_0_cf_1" pg:"pm10_0_cf_1"`
	P10um            float64 `json:"p_1_0_um" pg:"p_1_0_um"`
	PM10Atm          float64 `json:"pm1_0_atm" pg:"pm1_0_atm"`
	P25um            float64 `json:"p_2_5_um" pg:"p_2_5_um"`
	PM25Atm          float64 `json:"pm2_5_atm" pg:"pm2_5_atm"`
	P50um            float64 `json:"p_5_0_um" pg:"p_5_0_um"`
	PM100Atm         float64 `json:"pm10_0_atm" pg:"pm10_0_atm"`
	P100um           float64 `json:"p_10_0_um" pg:"p_10_0_um"`
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
	SensorId           string  `json:"SensorId" hass:"ignore" pg:"sensor_id"`                                    // MAC address of the device
	DateTime           string  `json:"DateTime" hass:"ignore"`                                                   // UTC datetime on the device
	Geo                string  `json:"Geo" hass:"ignore"`                                                        // name of the device
	Memory             int     `json:"Mem" hass:"ignore"`                                                        // free memory on the device?
	MemFrag            int     `json:"memfrag" hass:"ignore"`                                                    // ???
	MemFB              int     `json:"memfb" hass:"ignore"`                                                      // ???
	MemCS              int     `json:"memcs" hass:"ignore"`                                                      // ???
	Id                 int     `json:"Id" hass:"ignore"`                                                         // ???
	Latitude           float64 `json:"lat" hass:"ignore"`                                                        // configured latitude
	Longitude          float64 `json:"long" hass:"ignore"`                                                       // configure longitude
	ADC                float64 `json:"Adc" hass:"ignore"`                                                        // ???
	LoggingRate        int     `json:"loggingrate" hass:"ignore"`                                                // how often the data are updated
	Place              string  `json:"place" hass:"ignore"`                                                      // location of the device
	Version            string  `json:"version" hass:"ignore"`                                                    // firmware version of the device
	Uptime             int     `json:"uptime" hass:"ignore"`                                                     // number of seconds since last reboot
	RSSI               int     `json:"rssi" hass:"ignore" pg:"rssi"`                                             // wifi signal strength
	Period             int     `json:"period" hass:"ignore"`                                                     // number of seconds for averaging?
	HttpSuccess        int     `json:"httpsuccess" hass:"ignore"`                                                // number of successful HTTP requests
	HttpSends          int     `json:"httpsends" hass:"ignore"`                                                  // total number of http sends
	HardwareRevision   string  `json:"hardwarerevision" hass:"ignore"`                                           // version number of the physical hardware
	HardwareDiscovered string  `json:"hardwarediscovered" hass:"ignore"`                                         // list of the hardware present on the device
	Temperature        int     `json:"current_temp_f" hass-name:"temperature" hass-unit:"°F" pg:"temperature_f"` // current fahrenheit temperature rounded to nearest degree
	Humidity           int     `json:"current_humidity" hass-name:"humidity" hass-unit:"%" pg:"humidity"`        // current humidity rounded to nearest percent
	Dewpoint           int     `json:"current_dewpoint_f" hass-name:"dewpoint" hass-unit:"°F" pg:"dewpoint_f"`   // current dewpoint in fahrenheit rounded to nearest degree
	Pressure           float64 `json:"pressure" hass-name:"pressure" hass-unit:"mbar" pg:"pressure"`             // current pressure in millibar

	A                purpleAirMonitor `json:"sensor_a,omitempty" hass:"ignore"` // breakout for sensor a
	PM25AqiColor     string           `json:"p25aqic" hass:"ignore"`
	PM25Aqi          int              `json:"pm2.5_aqi" hass-unit:"AQI" hass-class:"AQI"`
	PM10Cf1          float64          `json:"pm1_0_cf_1" hass-unit:"µg/m³"`
	P03um            float64          `json:"p_0_3_um" hass-unit:"count"`
	PM25Cf1          float64          `json:"pm2_5_cf_1" hass-unit:"µg/m³"`
	P05um            float64          `json:"p_0_5_um" hass-unit:"count"`
	PM100Cf1         float64          `json:"pm10_0_cf_1 " hass-unit:"µg/m³"`
	P10um            float64          `json:"p_1_0_um" hass-unit:"count"`
	PM10Atm          float64          `json:"pm1_0_atm" hass-unit:"µg/m³"`
	P25um            float64          `json:"p_2_5_um" hass-unit:"count"`
	PM25Atm          float64          `json:"pm2_5_atm" hass-unit:"µg/m³"`
	P50um            float64          `json:"p_5_0_um" hass-unit:"count"`
	PM100Atm         float64          `json:"pm10_0_atm" hass-unit:"µg/m³"`
	P100um           float64          `json:"p_10_0_um" hass-unit:"count"`
	Key1Response     int              `json:"key1_response" hass:"ignore"`
	Key1ResponseDate int              `json:"key1_response_date" hass:"ignore"`
	Key1Count        int              `json:"key1_count" hass:"ignore"`
	TsLatency        int              `json:"ts_latency" hass:"ignore"`
	Key2Response     int              `json:"key2_response" hass:"ignore"`
	Key2ResponseDate int              `json:"key2_response_date" hass:"ignore"`
	Key2Count        int              `json:"key2_count" hass:"ignore"`
	TsSLatency       int              `json:"ts_s_latency" hass:"ignore"`

	B                 purpleAirMonitor `json:"sensor_b,omitempty" hass:"ignore"` // breakout for sensor b
	PM25AqiColorB     string           `json:"p25aqic_b" hass:"ignore"`
	PM25AqiB          int              `json:"pm2.5_aqi_b" hass-unit:"AQI" hass-class:"AQI"`
	PM10Cf1B          float64          `json:"pm1_0_cf_1_b" hass-unit:"µg/m³"`
	P03umB            float64          `json:"p_0_3_um_b" hass-unit:"count"`
	PM25Cf1B          float64          `json:"pm2_5_cf_1_b" hass-unit:"µg/m³"`
	P05umB            float64          `json:"p_0_5_um_b" hass-unit:"count"`
	PM100Cf1B         float64          `json:"pm10_0_cf_1_b" hass-unit:"µg/m³"`
	P10umB            float64          `json:"p_1_0_um_b" hass-unit:"count"`
	PM10AtmB          float64          `json:"pm1_0_atm_b" hass-unit:"µg/m³"`
	P25umB            float64          `json:"p_2_5_um_b" hass-unit:"count"`
	PM25AtmB          float64          `json:"pm2_5_atm_b" hass-unit:"µg/m³"`
	P50umB            float64          `json:"p_5_0_um_b" hass-unit:"count"`
	PM100AtmB         float64          `json:"pm10_0_atm_b" hass-unit:"µg/m³"`
	P100umB           float64          `json:"p_10_0_um_b" hass-unit:"count"`
	Key1ResponseB     int              `json:"key1_response_b" hass:"ignore"`
	Key1ResponseDateB int              `json:"key1_response_date_b" hass:"ignore"`
	Key1CountB        int              `json:"key1_count_b" hass:"ignore"`
	TsLatencyB        int              `json:"ts_latency_b" hass:"ignore"`
	Key2ResponseB     int              `json:"key2_response_b" hass:"ignore"`
	Key2ResponseDateB int              `json:"key2_response_date_b" hass:"ignore"`
	Key2CountB        int              `json:"key2_count_b" hass:"ignore"`
	TsSLatencyB       int              `json:"ts_s_latency_b" hass:"ignore"`

	PaLatency     int    `json:"pa_latency" hass:"ignore"`
	Response      int    `json:"response" hass:"ignore"`
	ResponseDate  int    `json:"response_date" hass:"ignore"`
	Latency       int    `json:"latency" hass:"ignore"`
	WirelessState string `json:"wlstate" hass:"ignore"`
	Status0       int    `json:"status_0" hass:"ignore"`
	Status1       int    `json:"status_1" hass:"ignore"`
	Status2       int    `json:"status_2" hass:"ignore"`
	Status3       int    `json:"status_3" hass:"ignore"`
	Status4       int    `json:"status_4" hass:"ignore"`
	Status5       int    `json:"status_5" hass:"ignore"`
	Status6       int    `json:"status_6" hass:"ignore"`
	Status7       int    `json:"status_7" hass:"ignore"`
	Status8       int    `json:"status_8" hass:"ignore"`
	Status9       int    `json:"status_9" hass:"ignore"`
	SSID          string `json:"ssid" hass:"ignore"`
}

// set up a global logger...
// see: https://stackoverflow.com/a/43827612/57626
var logger *log.Logger

var config tomlConfig

// var components tomlComponents
var client mqtt.Client

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	r := client.OptionsReader()
	logger.Infof("Connected to MQTT at %s", r.Servers())
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	logger.Errorf("MQTT Connection lost: %v", err)
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

	if config.Mqtt != (tomlConfigMQTT{}) {
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
		if config.Mqtt.TopicPrefix == "" {
			config.Mqtt.TopicPrefix = "purpleair"
		}
	} else {
		logger.Info("No MQTT configuration found - not publishing to MQTT broker")
		if config.Hass != (tomlConfigHass{}) {
			logger.Fatal("Hass configuration found but no MQTT configuration found - please configure MQTT broker")
		}
	}

	var ts *timescale
	if config.Timescale != (tomlConfigTimescale{}) {
		var err error
		if ts, err = newTimescale(context.Background(), config.Timescale); err != nil {
			logger.Fatal(err)
		}
		logger.Infof("Writing to TimescaleDB tables %s and %s", config.Timescale.Table, config.Timescale.SystemTable)
	}

	logger.Infof("HTTP Target: %s", config.PurpleAir.Url)
	var myClient = &http.Client{Timeout: 10 * time.Second}

	for {
		pastatus := new(purpleAirStatus)
		// see: https://stackoverflow.com/a/31129967/57626
		if err := getJson(config.PurpleAir.Url, pastatus, myClient); err != nil {
			logger.Warnf("Reading the PurpleAir failed, skipping this poll: %v", err)
			time.Sleep(time.Duration(config.PurpleAir.PollRate) * time.Second)
			continue
		}
		normalizePaStatus(pastatus)

		// if we don't set the specific topic, then we can grab and set the topic from the Geo field
		// this is useful if you're polling from multiple different sensors and aggregating them and
		// don't want to think about the topic name for each one

		if pastatus.Geo != "" {
			logger.Infof("Geo: %s", pastatus.Geo)
			logger.Infof("Sensor ID: %s", pastatus.SensorId)
			logger.Infof("Timestamp: %s", pastatus.DateTime)
			logger.Infof("Sensor 1 Color: %s", pastatus.PM25AqiColor)
			logger.Infof("Sensor 1 AQI: %d", pastatus.PM25Aqi)
			logger.Infof("Sensor 2 Color: %s", pastatus.PM25AqiColorB)
			logger.Infof("Sensor 2 AQI: %d", pastatus.B.PM25Aqi)

			if ts != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				err := ts.write(ctx, time.Now(), pastatus)
				cancel()
				switch {
				case errors.Is(err, errSchema):
					logger.Fatal(err)
				case err != nil:
					logger.Errorf("TimescaleDB write failed, %d+%d rows queued: %v", len(ts.monitors.queue), len(ts.system.queue), err)
				default:
					logger.Infof("Wrote reading to TimescaleDB")
				}
			}

			if config.Influx != (tomlConfigInflux{}) {
				write_influx(pastatus, &pastatus.A, &pastatus.B)
			}

			if config.Mqtt != (tomlConfigMQTT{}) {
				if config.Mqtt.Topic == "" {
					config.Mqtt.Topic = pastatus.Geo
				}
				publishMQTT(pastatus)
			}

			if config.Hass != (tomlConfigHass{}) {
				publishHass(pastatus)
			}
		} else {
			logger.Warnf("Got a strange response from the PurpleAir API - skipping this poll")
		}
		logger.Debugf("Sleeping for %d seconds", config.PurpleAir.PollRate)
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
		logger.Errorf("Error creating InfluxDB Client: %v", err)
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

	// An InfluxDB outage shouldn't take down the other outputs
	if err := c.Write(bp); err != nil {
		logger.Errorf("InfluxDB write failed: %v", err)
	}
}

func publishMQTT(status *purpleAirStatus) {
	v := reflect.ValueOf(*status)
	typeOfStatus := v.Type()

	for i := 0; i < v.NumField(); i++ {
		fieldName := typeOfStatus.Field(i).Name

		if fieldName == "A" || fieldName == "B" {
			continue
		}

		fieldValue := v.Field(i).Interface()
		topic := fmt.Sprintf("%s/%s/%s", config.Mqtt.TopicPrefix, config.Mqtt.Topic, fieldName)
		logger.Debugf("field[%s] = [%v]", fieldName, fieldValue)
		logger.Debugf("topic = %s", topic)
		token := client.Publish(topic, 0, false, fmt.Sprintf("%v", fieldValue))
		token.Wait()
	}

}

func publishHass(status *purpleAirStatus) {
	v := reflect.ValueOf(*status)
	typeOfStatus := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typeOfStatus.Field(i)
		fieldName := field.Name
		mqttFieldName := fieldName
		logger.Debugf("Field %s has type %s - %v", fieldName, field.Type, field.Tag)
		if hassTag, ok := field.Tag.Lookup("hass"); ok && hassTag == "ignore" {
			logger.Debugf("Ignoring sending field %s to HomeAssistant", fieldName)
			continue
		}
		if hassFieldName, ok := field.Tag.Lookup("hass-name"); ok {
			mqttFieldName = hassFieldName
		}

		// logger.Infof("Sending field %s to HomeAssistant", fieldName)

		// generage the topic name
		topic := fmt.Sprintf("%s/%s/%s/%s", config.Hass.DiscoveryPrefix, "sensor", config.Hass.DeviceName, mqttFieldName)

		// send the availabilty message
		token := client.Publish(topic+"/availability", 0, false, "online")
		token.Wait()

		// send the state message
		token = client.Publish(topic+"/state", 0, false, fmt.Sprintf("%v", reflect.ValueOf(status).Elem().FieldByName(fieldName).Interface()))
		token.Wait()

		// send the config message
		hassConfig := hassMqttConfig{
			AvailabilityTopic: topic + "/availability",
			ConfigTopic:       topic + "/config",
			Device: hassMqttConfigDevice{
				Identifiers:  []string{status.Geo},
				Manufacturer: config.Hass.Manufacturer,
				Model:        config.Hass.DeviceModel,
				Name:         config.Hass.DeviceName,
				SWVersion:    status.Version,
			},
			Name:       fieldName,
			Qos:        0,
			StateTopic: topic + "/state",
			UniqueId:   fmt.Sprintf("%s_%s", status.Geo, mqttFieldName),
		}

		if hassTag, ok := field.Tag.Lookup("hass-unit"); ok {
			hassConfig.UnitOfMeasurement = hassTag
		}

		configPayload, err := json.Marshal(hassConfig)
		if err != nil {
			logger.Errorf("Error marshalling hassConfig to JSON: %v", err)
			continue
		}
		token = client.Publish(topic+"/config", 0, false, configPayload)
		token.Wait()

	}

}
