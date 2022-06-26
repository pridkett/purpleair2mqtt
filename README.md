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

This program uses TOML as it's configuration file because all configuraiton file formats are terrible and TOML it terrible in the least amount of conflicting ways.

Running the Application
-----------------------

License
-------

Copyright (c) 2022 Patrick Wagstrom

Licensed under the terms of the MIT License
