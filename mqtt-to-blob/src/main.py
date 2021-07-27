from minio import Minio
import logging
import paho.mqtt.client as mqtt
import numpy as np
import os
import base64
import json
import datetime
import pytz
import cv2
import time

#with open("./../config.json", "r") as config_file:
#    config = json.load(config_file)

broker_url = os.environ['BROKER_URL']
print(broker_url)
broker_port = int(os.environ['BROKER_PORT'])
minio_url = os.environ['MINIO_URL']
minio_access_key = os.environ['MINIO_ACCESS_KEY']
minio_secret = os.environ['MINIO_SECRET_KEY']
#config = config["Europe/Vienna"]
topic = os.environ['TOPIC']
input_var = ""
timezone = pytz.timezone('Europe/Vienna')
logging.basicConfig(level=logging.DEBUG)
timestamp = ""

def on_connect(client, userdata, flags, rc):
    print("Connected With Result Code " + str(rc))

def on_message(client, userdata, message):
    print(message.payload)
    input_var = json.loads(message.payload)
    timestamp = datetime.datetime.fromtimestamp(int(input_var["timestamp_ms"])/1000.0, timezone).strftime("%Y%m%d_%H-%M-%S-%f")
    im = input_var["image"]
    im_bytes = base64.b64decode(im["image_bytes"])
    im_arr = np.frombuffer(im_bytes, dtype=np.uint8)  # im_arr is one-dim Numpy array
    img = cv2.imdecode(im_arr, flags=cv2.IMREAD_COLOR)
    img_saver = cv2.imwrite("./images/"+timestamp+".jpg", img)
    if img_saver==True:
        print("saved")
    else:
        print("failed to save")
    client = Minio(
        minio_url,
        access_key=minio_access_key,
        secret_key=minio_secret
    )
    found = client.bucket_exists("umhtest")
    if not found:
        client.make_bucket("umhtest")
    else:
        print("Bucket 'umhtest' already exists")
    # Upload 'file_path' as object name
    # Add 'file' to bucket 'umhtest'.
    client.fput_object(
        "umhtest", timestamp+".jpg", "./images/" + timestamp + ".jpg"
    )
    print(
        "'test_pic' is successfully uploaded as "
        "object 'photo' to bucket 'umhtest'."
    )
    if os.path.exists("./images/" + timestamp + ".jpg"):
        os.remove("./images/" + timestamp + ".jpg")
        print("file has been deleted")
    else:
        print("The file does not exist")

if __name__ == "__main__":
    print(broker_port)
    client = mqtt.Client()
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(broker_url, broker_port)
    client.subscribe(topic, qos=0)
    client.loop_forever()
