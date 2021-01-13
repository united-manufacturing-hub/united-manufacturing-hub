# STD imports
import json # for HTTP JSON
import os #for importing environment variables
import time

# 3rd party imports
import evdev
from evdev import InputDevice, categorize, ecodes
import paho.mqtt.client as mqtt

# Local imports

# Broker details
TRASMITTER_ID = os.environ['TRANSMITTERID']
BROKER_URL = os.environ['BROKER_URL']
BROKER_PORT = int(os.environ['BROKER_PORT'])
CUSTOMER_ID = os.environ['CUSTOMER_ID']
LOCATION = os.environ['LOCATION']
MACHINE_ID = os.environ['MACHINE_ID']
PUB_TOPIC = "ia/{}/{}/{}/barcode".format(CUSTOMER_ID,
                                        LOCATION,
                                        MACHINE_ID)

# The callback for when the client receives a CONNACK response from the server.
def on_connect(client, userdata, flags, rc):
   print(str(time.ctime(time.time())) + " Connected")

def on_disconnect(client, userdata, rc):
    if rc != 0:
        print(str(time.ctime(time.time())) + " Unexpected disconnection.")

# function to start up MQTT
def startMQTT():
    print(str(time.ctime(time.time())) + " Connecting...")
    mqttc = mqtt.Client(client_id=str(TRASMITTER_ID+str("_barcodereader")), clean_session=False)
    mqttc.on_connect = on_connect
    mqttc.on_disconnect = on_disconnect
    
    mqttc.connect_async(BROKER_URL, port=BROKER_PORT, keepalive=65535) # Keep alive for around 18h
    mqttc.loop_start()

    return mqttc

# Provided as an example taken from my own keyboard attached to a Centos 6 box:
scancodes = {
    # Scancode: ASCIICode
    0: None, 1: u'ESC', 2: u'1', 3: u'2', 4: u'3', 5: u'4', 6: u'5', 7: u'6', 8: u'7', 9: u'8',
    10: u'9', 11: u'0', 12: u'-', 13: u'=', 14: u'BKSP', 15: u'TAB', 16: u'q', 17: u'w', 18: u'e', 19: u'r',
    20: u't', 21: u'y', 22: u'u', 23: u'i', 24: u'o', 25: u'p', 26: u'[', 27: u']', 28: u'CRLF', 29: u'LCTRL',
    30: u'a', 31: u's', 32: u'd', 33: u'f', 34: u'g', 35: u'h', 36: u'j', 37: u'k', 38: u'l', 39: u';',
    40: u'"', 41: u'`', 42: u'LSHFT', 43: u'\\', 44: u'z', 45: u'x', 46: u'c', 47: u'v', 48: u'b', 49: u'n',
    50: u'm', 51: u',', 52: u'.', 53: u'/', 54: u'RSHFT', 56: u'LALT', 57: u' ', 100: u'RALT'
}

capscodes = {
    0: None, 1: u'ESC', 2: u'!', 3: u'@', 4: u'#', 5: u'$', 6: u'%', 7: u'^', 8: u'&', 9: u'*',
    10: u'(', 11: u')', 12: u'_', 13: u'+', 14: u'BKSP', 15: u'TAB', 16: u'Q', 17: u'W', 18: u'E', 19: u'R',
    20: u'T', 21: u'Y', 22: u'U', 23: u'I', 24: u'O', 25: u'P', 26: u'{', 27: u'}', 28: u'CRLF', 29: u'LCTRL',
    30: u'A', 31: u'S', 32: u'D', 33: u'F', 34: u'G', 35: u'H', 36: u'J', 37: u'K', 38: u'L', 39: u':',
    40: u'\'', 41: u'~', 42: u'LSHFT', 43: u'|', 44: u'Z', 45: u'X', 46: u'C', 47: u'V', 48: u'B', 49: u'N',
    50: u'M', 51: u'<', 52: u'>', 53: u'?', 54: u'RSHFT', 56: u'LALT',  57: u' ', 100: u'RALT'
}
#setup vars
x = ''
caps = False

def find_device():
  device_names = ['VirtualBox USB Tablet', 'VirtualBox mouse integration', 'Video Bus', 'ImExPS/2 Generic Explorer Mouse', 'AT Translated Set 2 keyboard', 'Sleep Button', 'Power Button']
  devices = [evdev.InputDevice(fn) for fn in evdev.list_devices()]
  device = None
  for d in devices:
    print(str(time.ctime(time.time())) + "Device " + d.name)
    if d.name not in device_names:
      print(str(time.ctime(time.time())) + "Found device " + d.name)
      device = d
  return device

while True:
    mqttc = startMQTT() # start MQTT
    while True:
        dev = find_device()

        if dev != None:
            #grab provides exclusive access to the device
            dev.grab()

            #loop
            for event in dev.read_loop():
                if event.type == ecodes.EV_KEY:
                    data = categorize(event)  # Save the event temporarily to introspect it
                    if data.scancode == 42:
                        if data.keystate == 1:
                            caps = True
                        if data.keystate == 0:
                            caps = False
                    if data.keystate == 1:  # Down events only
                        if caps:
                            key_lookup = u'{}'.format(capscodes.get(data.scancode)) or u'UNKNOWN:[{}]'.format(data.scancode)  # Lookup or return UNKNOWN:XX
                        else:
                            key_lookup = u'{}'.format(scancodes.get(data.scancode)) or u'UNKNOWN:[{}]'.format(data.scancode)  # Lookup or return UNKNOWN:XX
                        if (data.scancode != 42) and (data.scancode != 28):
                            x += key_lookup
                        if(data.scancode == 28):

                            timestamp_ms = int(round(time.time() * 1000))

                            # store values in payload
                            payload = {
                                "serial_number": TRASMITTER_ID+"_barcodereader",
                                "timestamp_ms":timestamp_ms,
                                "barcode": x
                            }

                            #Send values
                            (result, mid) = mqttc.publish(PUB_TOPIC, payload=json.dumps(payload), qos=1)

                            if (result != 0):
                                print(str(time.ctime(time.time())) + " [Error in publish] " + str(result))

                            print(str(time.ctime(time.time())) + " " + str(json.dumps(payload)))          # Print it all out!
                            x = ''
        else:
            print("No device found. Trying again in 10 seconds...")
        time.sleep(10)

