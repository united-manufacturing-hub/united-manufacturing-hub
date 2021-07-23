import logging
from typing import List
from array import *
import paho.mqtt.client as mqtt
import numpy as np
import os
import base64
import json
import datetime
import pytz
import cv2
import time
import sys
import yaml
from azure.storage.blob import ContainerClient

with open("./../config/config.json", "r") as config_file:
    config = json.load(config_file)

config = config["LISTENER"]

topic = "ia/rawImage/#"
input_var = ""
timezone = pytz.timezone(config["TIMEZONE"])
logging.basicConfig(level=logging.DEBUG)

def on_connect(client, userdata, flags, rc):
    print("Connected With Result Code " + str(rc))

def on_message(client, userdata, message):
    input_var = json.loads(message.payload)
    timestamp = datetime.datetime.fromtimestamp(int(input_var["timestamp_ms"])/1000.0, timezone).strftime("%Y%m%d_%H-%M-%S-%f")
    im = input_var["image"]
    im_bytes = base64.b64decode(im["image_bytes"])
    im_arr = np.frombuffer(im_bytes, dtype=np.uint8)  # im_arr is one-dim Numpy array
    img = cv2.imdecode(im_arr, flags=cv2.IMREAD_COLOR)
    #print("image")

def


client = mqtt.Client()
client.on_connect = on_connect
client.on_message = on_message
client.connect("localhost", 1883)

client.subscribe(topic, qos=0)

client.loop_forever()


"""
#upload file to azurestorage. Storage configured in config file
def load_config():
    dir_root = os.path.dirname(os.path.abspath(__file__))
    with open(dir_root + "/config.yaml", "r") as yamlfile:
        return yaml.load(yamlfile, Loader=yaml.FullLoader)

def get_files(dir):
    with os.scandir(dir) as entries:
        for entry in entries:
            if entry.is_file() and not entry.name.startswith('.'):
                yield entry

def upload(files, connection_string, container_name):
    container_client = ContainerClient.from_connection_string(connection_string, container_name)
    print("Uploading files to blob storage...")

    for file in files:
        blob_client = container_client.get_blob_client(file.name)
        with open(file.path, "rb") as data:
            blob_client.upload_blob(data)
            print(f'{file.name} uploaded to blob storage')
            os.remove(file)
            print(f'{file} removed from folder')

config = load_config()
pictures = get_files(config["source_folder"]+"/images")
upload(pictures, config["azure_storage_connectionstring"], config["pictures_container_name"])
"""

""" 
###TODO###
+Abspeichern des JPGs mit der image UID
"""