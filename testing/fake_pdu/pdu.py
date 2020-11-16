#! /usr/bin/env python3
# Copyright 2020 Hewlett Packard Enterprise Development LP
# Fake PDU is a simple script that servers a static mockup of a ServerTech PDU

from flask import Flask
from flask import json
from flask import abort
from flask import request
import time

api = Flask(__name__)

config_info_units = None
config_outlets = None
config_groups = None
monitor_branches = None
monitor_lines = None
monitor_outlets = None
monitor_phases = None
monitor_units = None
monitor_sensors_temp = None
monitor_sensors_humidity = None

with open('./data/config_info_units.json') as f:
    config_info_units = json.load(f)
with open('./data/config_outlets.json') as f:
    config_outlets = json.load(f)
with open('./data/config_groups.json') as f:
    config_groups = json.load(f)
with open('./data/monitor_branches.json') as f:
    monitor_branches = json.load(f)
with open('./data/monitor_lines.json') as f:
    monitor_lines = json.load(f)
with open('./data/monitor_outlets.json') as f:
    monitor_outlets = json.load(f)
with open('./data/monitor_phases.json') as f:
    monitor_phases = json.load(f)
with open('./data/monitor_units.json') as f:
    monitor_units = json.load(f)
with open('./data/monitor_sensors_temp.json') as f:
    monitor_sensors_temp = json.load(f)
with open('./data/monitor_sensors_humidity.json') as f:
    monitor_sensors_humidity = json.load(f)


def sleepy():
    # Emulate the time it takes for the PDU to respond to requests
    # TODO Make this "random/noisy"
    time.sleep(0.5)

def get_element(collection, wanted_id):
    for element in collection:
        if element['id'] == wanted_id:
            return element
    
    abort(404)

@api.route('/jaws/config/info/units')
def get_config_info_units():
    sleepy()
    return json.dumps(config_info_units)

@api.route('/jaws/config/outlets')
def get_config_outlets():
    sleepy()
    return json.dumps(config_outlets)

@api.route('/jaws/config/groups')
def post_config_groups():
    sleepy()
    return json.dumps(config_groups)

@api.route('/jaws/config/groups/<id>',  methods=['POST'])
def post_config_groups_id(id):
    return "", 201

@api.route('/jaws/config/outlets/<id>')
def get_config_outlets_id(id):
    sleepy()
    return get_element(config_outlets, id)

@api.route('/jaws/control/outlets/<id>', methods=['GET', 'PATCH'])
def get_control_outlets_id(id):
    return "", 204 #StatusNoContent

@api.route('/jaws/monitor/branches')
def get_monitor_branches():
    sleepy()
    return json.dumps(monitor_branches)

@api.route('/jaws/config/branches/<id>')
def get_monitor_branches_id(id):
    sleepy()
    return get_element(monitor_branches, id)

@api.route('/jaws/monitor/lines')
def get_monitor_lines():
    sleepy()
    return json.dumps(monitor_lines)

@api.route('/jaws/monitor/lines/<id>')
def get_monitor_lines_id(id):
    sleepy()
    return get_element(monitor_lines, id)

@api.route('/jaws/monitor/outlets')
def get_monitor_outlets():
    sleepy()
    return json.dumps(monitor_outlets)

@api.route('/jaws/monitor/outlets/<id>')
def get_monitor_outlets_id(id):
    sleepy()
    return get_element(monitor_outlets, id)

@api.route('/jaws/monitor/phases')
def get_monitor_phases():
    sleepy()
    return json.dumps(monitor_phases)

@api.route('/jaws/monitor/phases/<id>')
def get_monitor_phases_id(id):
    sleepy()
    return get_element(monitor_phases, id)

@api.route('/jaws/monitor/units')
def get_monitor_units():
    sleepy()
    return json.dumps(monitor_units)

@api.route('/jaws/monitor/units/<id>')
def get_monitor_units_id(id):
    sleepy()
    return get_element(monitor_units, id)

@api.route('/jaws/monitor/sensors/temp')
def get_monitor_sensors_temp():
    sleepy()
    return json.dumps(monitor_sensors_temp)

@api.route('/jaws/monitor/sensors/temp/<id>')
def get_monitor_sensors_temp_id(id):
    sleepy()
    return get_element(monitor_sensors_temp, id)

@api.route('/jaws/monitor/sensors/humidity')
def get_monitor_sensors_humidity():
    sleepy()
    return json.dumps(monitor_sensors_humidity)

@api.route('/jaws/monitor/sensors/humidity/<id>')
def get_monitor_sensors_humidity_id(id):
    sleepy()
    return get_element(monitor_sensors_humidity, id)

if __name__ == '__main__':
    api.run(debug=False, host='0.0.0.0', port=8090)