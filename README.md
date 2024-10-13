purpleair2mqtt
==============

Patrick Wagstrom &lt;160672+pridkett@users.noreply.github.com&gt;<br>
June 2022

Overview
--------

This program connects to local web server on a PurpleAir air quality monitor
and publishes the data to an MQTT broker and optionally logs the data into an
influxdb database for further analysis. There are already a few libraries out there that connect the PurpleAir API, but I'm the kind of person that wants local interfaces to local devices

Background
----------

Once you have your PurpleAir monitor connected to your local network, you can access it by just going to its IP address and making an HTTP request. This provides a user friendly page of what is going on with the monitor. If you'd like to get the data structured, make a request to `/json`.

This provides an excellent way to get real-time information from a local device without needing to manage API keys or even make calls out to the public internet. It's this JSON payload that the program parses.

Configuration
-------------

This program uses TOML as it's configuration file because all configuraiton file formats are terrible and TOML it terrible in the least amount of conflicting ways. You'll need to figure out the IP address or hostname of your Purple Air monitor on your local network.

First, define the location of the device and the polling rate in seconds. I find that you don't need to read it much more than once every two minutes. This will allow the application to pull the information from the Purple Air sensor.

```toml
[purpleair]
    url = "http://192.168.1.24/json"
    poll_rate = 120

```

Next, define the information needed for wherever your MQTT broker is running. I haven't done anything fancy here, but if it helps, I use Mosquitto as my MQTT broker. You'll need the hostname. I don't use any username or password here. If `topic_prefix` is left as null it will default to `airquality` and if `topic` is left as null it will default to the `geo` identifier of your PurpleAir sensor.

```toml
[mqtt]
    broker_host = "mqttbroker.local"
    broker_port = 1883
    client_id = "purpleair2mqtt"
    topic_prefix = "airquality"
    topic = ""
```

If you want Home Assistant integration, this chunk _should_ work to support Home Assistant auto discovery from MQTT. I need to do some more testing to see how well it works. I don't actually use Home Assistant that much.

```toml
[hass]
    discovery = true
    discovery_prefix = "homeassistant"
    device_model = "pa-sd-ii"
    device_name = "pa-sd-ii"
    # if you don't set object_id then you'll get end up with the MAC as your id
    object_id = "pa-sd-ii"
```

Finally, if you'd like to use the native InfluxDB integration, this section should work for you. You'll need to supply the `hostname`, create the database, which defaults to `purpleair` and define the username and password to write to that database.

```toml
[influx]
    hostname = "influxdb.local"
    port = 8086
    database = "purpleair"
    username = "YOUR_USERNAME"
    password = "YOUR_PASSWORD"
```

Building the Application
------------------------

This should be able to be built like most other straightforward golang applications.

```bash
go build
```

Running the Application
-----------------------

```bash
./purpleair2mqtt -config config.toml
```

Grafana Integration
-------------------

This application has some lightweight Grafana integration, but it's not what I'd call fancy. I'll document that more in the future.

Running with Docker
-------------------

One of the advantages of running this application with Docker is that you can use `docker-compose` and then not worry about needing to restart the system all the time. This makes this way less of a concern when problems happen.

### Building the Container

```bash
docker build -t pridkett/purpleair2mqtt .
```

### Running the Container as a One Off

### Running the Container from `docker-compose`

License
-------

Copyright (c) 2022 Patrick Wagstrom

Licensed under the terms of the MIT License
