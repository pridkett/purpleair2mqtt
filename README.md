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

This program uses TOML as it's configuration file because all configuraiton file formats are terrible and TOML it terrible in the least amount of conflicting ways. Here's a sample configuraiton file:

```toml
[purpleair]
    url = "http://IP_ADDRESS_OR_HOSTNAME_OF_YOUR_PURPLE_AIR_DEVICE/json"
    poll_rate = 120 # by default, without ?live=true in the URL, it only updates ever 2 minutes

[mqtt]
    broker_host = "IP_ADDRESS_OR_HOSTNAME_OF_YOUR_MQTT_BROKER"
    broker_port = 1883
    client_id = "purpleair2mqtt"
    topic_prefix = "purpleair"
    topic = ""

# NOTE: This will overwrite the existing topic prefix
[hass]
    discovery = true
    discovery_prefix = "homeassistant"
    device_model = "pa-sd-ii"
    device_name = "pa-sd-ii"
    # if you don't set object_id then you'll get end up with the MAC as your id
    object_id = "pa-sd-ii"

[influx]
    hostname = "IP_ADDRESS_OR_HOSTNAME_OF_YOUR_INFLUXDB_SERVER"
    port = 8086
    database = "purpleair"
    username = "purpleair"
    password = "INFLUXDB_PASSWORD"
```

Of note, for the InfluxDB support, it only supports InfluxDB version 1 and you need to create the database and user ahead of time as the program will not create them for you.

Running the Application
-----------------------

If you're running this locally, you can run this command:

```bash
./purpleair2mqtt -config /path/to/config.toml
```

I normally run the program via `docker compose`. You can use the existing `docker-compose.example.yaml` file and modify it as needed. Then you should have the advantage of seeing the process restart automatically.

License
-------

Copyright (c) 2022 Patrick Wagstrom

Licensed under the terms of the MIT License
