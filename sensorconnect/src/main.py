# -- coding: utf-8 --
"""
Created on 2019-09-02
@author: Jeremy Theocharis
"""

#imports
import time
import random
import datetime
import paho.mqtt.client as mqtt
from struct import *
import ipaddress
import sys, traceback

#from thread import start_new_thread # threading
import requests # HTTP requests
import json # for HTTP JSON
import threading

import os #for importing environment variables

# for parsing IODD files
import xml.etree.ElementTree as ET
from collections import defaultdict
import re
import glob

# Broker details
transmitterID = os.environ['TRANSMITTERID']
broker_url = os.environ['BROKER_URL']
broker_port = int(os.environ['BROKER_PORT'])
ip_range = os.environ['IP_RANGE']

# Get timestamp. Saves a lot of code
def get_timestamp():
    return time.ctime(time.time())

def etree_to_dict(t): ## Stackoverflow kommenatere einfügen
    d = {t.tag: {} if t.attrib else None}
    children = list(t)
    if children:
        dd = defaultdict(list)
        for dc in map(etree_to_dict, children):
            for k, v in dc.items():
                dd[k].append(v)
        d = {t.tag: {k: v[0] if len(v) == 1 else v
                     for k, v in dd.items()}}
    if t.attrib:
        d[t.tag].update(('@' + k, v)
                        for k, v in t.attrib.items())
    if t.text:
        text = t.text.strip()
        if children or t.attrib:
            if text:
              d[t.tag]['#text'] = text
        else:
            d[t.tag] = text
    return d

# read in all IODD files
deviceDataArray = []
for filename in glob.glob(os.path.join('./iodd_files', '*.xml')):
   with open(filename, 'r') as f: # open in readonly mode
    xmlstring =f.read()

    xmlstring = re.sub(' xmlns="[^"]+"', '', xmlstring, count=1)
    root = ET.fromstring(xmlstring)

    # prepare final array
    processDataForDeviceArray = []

    # get device id
    DeviceIdentity = root.find("ProfileBody/DeviceIdentity")
    DeviceIdentityDict = etree_to_dict(DeviceIdentity)
    deviceId=DeviceIdentityDict['DeviceIdentity']['@deviceId']
    vendorId=DeviceIdentityDict['DeviceIdentity']['@vendorId']

    # processTranslations
    translationsDict = {}

    translations = root.findall("ExternalTextCollection/PrimaryLanguage/Text")
    for translation in translations:
      translation = etree_to_dict(translation)
      primary = translation['Text']['@id']
      translation = translation['Text']['@value']
      translationsDict.update({primary:translation})

    # process data
    processData = root.find("ProfileBody/DeviceFunction/ProcessDataCollection/ProcessData")
    processDataDict = etree_to_dict(processData)

    bitLength = processDataDict['ProcessData']['ProcessDataIn']['Datatype']['@bitLength']
    datapoints = processDataDict['ProcessData']['ProcessDataIn']['Datatype']['RecordItem']

    for recordItem in datapoints:
      print(recordItem)
      name = recordItem['Name']['@textId']
      translatedName = translationsDict[name]
      datatype = recordItem['SimpleDatatype']['@{http://www.w3.org/2001/XMLSchema-instance}type']

      if (datatype == "BooleanT"):
        length = 1
      elif (datatype == "OctetStringT"):
        length = recordItem['SimpleDatatype']['@fixedLength']*8
      else:
        length = recordItem['SimpleDatatype']['@bitLength']

      offset = recordItem['@bitOffset']

      deviceDict = {
        "name": translatedName,
        "datatype": datatype,
        "length": length,
        "offset": offset
      }

      processDataForDeviceArray.append(deviceDict)

    tempdict = {
      "vendorId": int(vendorId),
      "devices": [
        {
          "deviceId": int(deviceId),
          "content": processDataForDeviceArray
        }
      ]
    }

    deviceDataArray.append(tempdict)
# getDeviceData returns the device data for a given deviceData array, vendorId and deviceId
def getDeviceData(deviceData, vendorId, deviceId): ##
    if deviceData == None:
        return None
    for vendor in deviceData:
        if int(vendor['vendorId']) == vendorId:
            if vendor['devices'] == None:
                return None
            for device in vendor['devices']:
                if int(device['deviceId']) == deviceId:
                    if device['content'] == None:
                        return None
                    return device['content']
    return None

# The callback for when the client receives a CONNACK response from the server.
def on_connect(client, userdata, flags, rc):
   print(str(get_timestamp()) + " MQTT Connected")

def on_disconnect(client, userdata, rc):
    if rc != 0:
        print(str(get_timestamp()) + " Unexpected disconnection.")

# function to start up MQTT
def startMQTT():
    mqttc = mqtt.Client(client_id=str(transmitterID)+"_ia_sensorconnect",
                        clean_session=False)
    mqttc.on_connect = on_connect
    mqttc.on_disconnect = on_disconnect

    mqttc.connect_async(broker_url, port=broker_port, keepalive=65535) # Keep alive for around 18h

    mqttc.loop_start()

    return mqttc

# find ifm devices
def discoverDevices():
    returnArray = []
    print(str(get_timestamp()) + " Searching for new devices")

    # Payload to send to the gateways
    payload = """{
        "code":"request",
        "cid":-1,
        "adr":"/getdatamulti",
        "data":{
            "datatosend":[
                "/deviceinfo/serialnumber/","/deviceinfo/productcode/"]
        }
    }"""

    # Iterate over the specified IP space to find connected gateways
    for ip in ipaddress.IPv4Network(ip_range): #IPv4Network('172.16.0.0/24'):
        try:
            url = str(ip)
            r = requests.post("http://"+url, data=payload, timeout=0.1)

            r.raise_for_status()

            response = json.loads(r.text)

            productCode = response["data"]["/deviceinfo/productcode/"]["data"]
            serialNumber = response["data"]["/deviceinfo/serialnumber/"]["data"]
            returnArray.append([url, productCode, serialNumber])
        except Exception as e:
            pass

    return returnArray

def dataProcessing(data, modeArray, portNumber, mqttc, serialNumber):
    timestamp_ms = int(round(time.time() * 1000))

    try:
        for i in range (1,portNumber+1):
            # All publications share the same root topic
            mqtt_raw_topic = "ia/raw/{}/{}/X0{}".format(transmitterID, serialNumber, i)

            mode = modeArray[i-1] # get port mode (Digital Input, Digital Output or IO-Link)
            if (mode == 1):
                port_mode = "DI"
                connected = 1
                primaryvalue = int(data["data"]["/iolinkmaster/port["+str(i)+"]/pin2in"]["data"])

                # store values in payload
                payload = {
                    "serial_number": serialNumber+"-X0"+str(i),
                    "timestamp_ms":timestamp_ms,
                    "type": port_mode,
                    "connected": connected,
                    "value": primaryvalue
                }

                # send out result
                (result, _) = mqttc.publish(mqtt_raw_topic + "/DI", payload=json.dumps(payload), qos=1)

                if (result != 0):
                    print(str(get_timestamp()) + " [Error in publish] " + str(result))

            elif (mode == 2):
                port_mode = "DO"
                connected = 1

                # store values in payload
                payload = {
                    "serial_number": serialNumber+"-X0"+str(i),
                    "timestamp_ms":timestamp_ms,
                    "type": port_mode,
                    "connected": connected
                }

                # send out result
                (result, _) = mqttc.publish(mqtt_raw_topic + "/DO", payload=json.dumps(payload), qos=1)

                if (result != 0):
                    print(str(get_timestamp()) + " [Error in publish] " + str(result))

            elif (mode == 3):
                port_mode = "IO-Link"
                connection = data["data"]["/iolinkmaster/port["+str(i)+"]/iolinkdevice/pdin"]["code"]
                if (connection == 200): # if return code is 200, then an IO-link device is connected
                    connected = 1
                    deviceID = data["data"]["/iolinkmaster/port["+str(i)+"]/iolinkdevice/deviceid"]["data"]
                    vendorID = data["data"]["/iolinkmaster/port["+str(i)+"]/iolinkdevice/vendorid"]["data"]
                    value_string = data["data"]["/iolinkmaster/port["+str(i)+"]/iolinkdevice/pdin"]["data"]

                    # store values in payload
                    payload = {
                        "serial_number": serialNumber+"-X0"+str(i),
                        "timestamp_ms":timestamp_ms,
                        "type": port_mode,
                        "connected": connected
                    }

                    tempVendorId = vendorID
                    tempDeviceId = deviceID
                    tempValueString = value_string
                    tempValueStringLength = len(tempValueString)

                    tempDeviceData = getDeviceData(deviceDataArray,tempVendorId,tempDeviceId)

                    if tempDeviceData == None: #if device is not known
                            datapointDict = {"value_string": tempValueString}
                            payload.update(datapointDict)
                            # send out result
                            (result, _) = mqttc.publish(mqtt_raw_topic + "/"+str(tempVendorId)+"-"+str(tempDeviceId), payload=json.dumps(payload), qos=1)
                            if (result != 0):
                                print(str(get_timestamp()) + " [Error in publish] " + str(result))
                            continue
                    else:
                        tempDeviceData = tempDeviceData[::-1] #no idea what this line does, but tempDeviceData is not allowed to be None

                    try:
                        scale = 16
                        num_of_bits = tempValueStringLength*4
                        binArray = bin(int(tempValueString, scale))[2:].zfill(num_of_bits)

                        for datapoint in tempDeviceData:
                            tempName = datapoint["name"]
                            tempDatatype = datapoint["datatype"]
                            tempOffset = int(datapoint["offset"])
                            tempLength = int(datapoint["length"])

                            firstCut = tempValueStringLength*4-tempLength-tempOffset
                            secondCut = tempValueStringLength*4-tempOffset
                            if tempDatatype == "OctetStringT":
                                value=str(hex(int(binArray[firstCut:secondCut],2))) # show strings as HEX
                            else:
                                value = int(binArray[firstCut:secondCut],2)

                            datapointDict = {tempName: value}

                            payload.update(datapointDict)

                        # send out result
                        (result, _) = mqttc.publish(mqtt_raw_topic + "/"+str(tempVendorId)+"-"+str(tempDeviceId), payload=json.dumps(payload), qos=1)

                        if (result != 0):
                            print(str(get_timestamp()) + " [Error in publish] " + str(result))
                    except Exception as e:
                        print(str(get_timestamp()) + " [Error in dataProcessing of device"+str(tempDeviceId)+"]" + str(e))

                else:
                    connected = 0
                    # store values in payload
                    payload = {
                        "serial_number": serialNumber+"-X0"+str(i),
                        "timestamp_ms":timestamp_ms,
                        "type": port_mode,
                        "connected": connected
                    }

                    # send out result
                    (result, _) = mqttc.publish(mqtt_raw_topic + "/IO-Link", payload=json.dumps(payload), qos=1)

                    if (result != 0):
                        print(str(get_timestamp()) + " [Error in publish] " + str(result))

    except Exception as e:
        print(str(get_timestamp()) + " [Error in dataProcessing]" + str(e))
        traceback.print_exc(file=sys.stdout)
        print(str(get_timestamp()) + " Data: " + str(data))

def readOutDevice(device, mqttc):
    while True: # never jump out of routine
        device_ip = device[0]
        device_type = device[1]
        device_serial_number = device[2]

        portNumber = 4
        if (device_type == "AL1342" or device_type == "AL1352" or device_type == "AL1353"):
            portNumber = 8

        # get port modes
        url = device_ip

        # change payload according to transmitterType
        payload = """{"code":"request","cid":-1,"adr":"/getdatamulti","data":{"datatosend":["/iolinkmaster/port[1]/mode" """
        for i in range (2,portNumber+1):
          payload = payload + ""","/iolinkmaster/port[""" +str(i)+"""]/mode" """
        payload = payload + """]}}"""

        r = requests.post("http://"+url, data=payload)
        r.raise_for_status()
        initResponse = json.loads(r.text)

        #store modes
        modeArray = []
        for i in range (1,portNumber+1):
            modeArray.append(initResponse["data"]["/iolinkmaster/port["+str(i)+"]/mode"]["data"])

        # read out values
        while True:
            try:
                # change payload according to transmitter type
                payload = """{"code":"request","cid":-1,"adr":"/getdatamulti","data":{"datatosend":["/iolinkmaster/port[1]/iolinkdevice/deviceid","/iolinkmaster/port[1]/iolinkdevice/pdin","/iolinkmaster/port[1]/iolinkdevice/vendorid","/iolinkmaster/port[1]/pin2in" """
                for i in range (2,portNumber+1):
                  payload = payload + ""","/iolinkmaster/port[""" +str(i)+"""]/iolinkdevice/deviceid","/iolinkmaster/port[""" +str(i)+"""]/iolinkdevice/pdin","/iolinkmaster/port[""" +str(i)+"""]/iolinkdevice/vendorid","/iolinkmaster/port[""" +str(i)+"""]/pin2in" """
                payload = payload + """]}}"""

                r = requests.post("http://"+url, data=payload)

                r.raise_for_status()

                response = json.loads(r.text)

                dataProcessing(response,modeArray, portNumber, mqttc, device_serial_number)

            except Exception as e:
                print(str(get_timestamp()) + " [Error in readOutDevice]" + str(e))

        print(str(get_timestamp()) + " [Error] device with IP-address " + str(device_ip) + " encountered an error. Trying again in 10 seconds...")
        time.sleep(10)

while True:
    mqttc = startMQTT() # start MQTT
    devices = [] # All connected gateways
    functionThreads = []

    while True:
        # Discover devices
        connected_devices = discoverDevices()

        # Find new devices
        new_devices = [item for item in connected_devices if item not in devices]
        print("{} {} devices discovered. {} of them are new.".format(get_timestamp(), len(connected_devices), len(new_devices)))

        # startup thread for each found device
        for device in new_devices:
            device_ip = device[0]
            device_type = device[1]
            device_serial_number = device[2]

            print("{} Starting thread for: {} | {} | {}".format(get_timestamp(), device_ip, device_type, device_serial_number))
            thread = threading.Thread(target = readOutDevice, args = (device, mqttc))
            thread.start()
            functionThreads.append(thread) # add to thread list
            devices.append(device) # add to device list
        print(str(get_timestamp()) + " Waiting 10 seconds till next discover")
        time.sleep(10)
