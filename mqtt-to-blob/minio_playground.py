from minio import Minio
from minio.error import S3Error
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
#from azure.storage.blob import ContainerClient

with open("./config.json", "r") as config_file:
    config = json.load(config_file)

config = config["LISTENER"]

topic = "ia/rawImage/#"
input_var = ""
timezone = pytz.timezone(config["TIMEZONE"])
logging.basicConfig(level=logging.DEBUG)
timestamp = ""

def on_connect(client, userdata, flags, rc):
    print("Connected With Result Code " + str(rc))

def on_message(client, userdata, message):
    #print("message_received")
    input_var = json.loads(message.payload)
    timestamp = datetime.datetime.fromtimestamp(int(input_var["timestamp_ms"])/1000.0, timezone).strftime("%Y%m%d_%H-%M-%S-%f")
    im = input_var["image"]
    im_bytes = base64.b64decode(im["image_bytes"])
    im_arr = np.frombuffer(im_bytes, dtype=np.uint8)  # im_arr is one-dim Numpy array
    img = cv2.imdecode(im_arr, flags=cv2.IMREAD_COLOR)
    print(timestamp)
    img_saver=cv2.imwrite("./images/"+timestamp+".jpg", img)
    #if img_saver==True:
    #    print("saved")
    #else:
    #    print("failed to save")

def main():
    # Create a client with the MinIO server playground, its access key
    # and secret key.
    print("timestamp")
    client = Minio(
        "play.min.io",
        access_key="Q3AM3UQ867SPQQA43P2F",
        secret_key="zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG",
    )

    # Make umh test bucket if not exist.
    found = client.bucket_exists("testumh")
    if not found:
        client.make_bucket("testumh")
    else:
        print("Bucket 'testumh' already exists")

    # Upload 'file_path' as object name
    # Add 'file' to bucket 'testumh'.
    client.fput_object(
        "testumh", timestamp+".jpg", "./images/" + timestamp + ".jpg",
    )
    print(
        "'test_pic' is successfully uploaded as "
        "object 'photo' to bucket 'testumh'."
    )

if __name__ == "__main__":
    try:
        main()
    except S3Error as exc:
        print("error occurred.", exc)

client = mqtt.Client()
client.on_connect = on_connect
client.on_message = on_message
client.connect("localhost", 1883)
client.subscribe(topic, qos=0)
client.loop_forever()