from minio import Minio
import logging
import paho.mqtt.client as mqtt
import numpy as np
import os
import base64
import json
import cv2

#Settig up the env variables, see index.md for further explanation
LOGGING_LEVEL = os.environ.get('LOGGING_LEVEL', 'INFO')
broker_url = os.environ['BROKER_URL']
broker_port = int(os.environ['BROKER_PORT'])
minio_url = os.environ['MINIO_URL']
minio_access_key = os.environ['MINIO_ACCESS_KEY']
minio_secret = os.environ['MINIO_SECRET_KEY']
bucket_name = os.environ['BUCKET_NAME']
topic = os.environ['TOPIC']
image_uid = os.environ['IMAGE_UID']
image_bytes = os.environ['IMAGE_BYTES']
input_var = ""

#Connects to the mqtt client. If you want to be sure that the connection attempt was successful then see the logs.
#This function will return the return code (rc) 0 if connected successfully.
#The following values are possible: 0 - Connection successful; 1 - Connection refused – incorrect protocol version; 2 - Connection refused – invalid client identifier;
#3 - Connection refused – server unavailable; 4 - Connection refused – bad username or password; 5 - Connection refused – not authorised; 6-255 - Currently unused.

def on_connect(client, userdata, flags, rc):
    logging.info("Connected With Result Code " + str(rc))

#Message is an object and the payload property contains the message data which is binary data.

def on_message(client, userdata, message):
    input_var = json.loads(message.payload)
    im = input_var["image"]
    uid = input_var[image_uid]
    im_bytes = base64.b64decode(im[image_bytes])
    im_arr = np.frombuffer(im_bytes, dtype=np.uint8)
    img = cv2.imdecode(im_arr, flags=cv2.IMREAD_COLOR)
    img_saver = cv2.imwrite("./images/"+uid+".jpg", img)
    if img_saver==True:
        logging.info("saved")
    else:
        logging.debug("failed to save image to cache")
    client = Minio(
        minio_url,
        access_key=minio_access_key,
        secret_key=minio_secret
    )
    found = client.bucket_exists(bucket_name)
    if not found:
        client.make_bucket(bucket_name)
    else:
        logging.info("Bucket already exists")
    client.fput_object(
        bucket_name, uid +".jpg", "./images/" + uid + ".jpg"
    )
    logging.info("Successfully uploaded")

    if os.path.exists("./images/" + uid + ".jpg"):
        os.remove("./images/" + uid + ".jpg")
        logging.info("file has been deleted")
    else:
        logging.info("The file does not exist")

if __name__ == "__main__":

    if LOGGING_LEVEL == "DEBUG":
        logging.basicConfig(level=logging.DEBUG)
    elif LOGGING_LEVEL == "INFO":
        logging.basicConfig(level=logging.INFO)
    elif LOGGING_LEVEL == "WARNING":
        logging.basicConfig(level=logging.WARNING)
    elif LOGGING_LEVEL == "ERROR":
        logging.basicConfig(level=logging.ERROR)
    elif LOGGING_LEVEL == "CRITICAL":
        logging.basicConfig(level=logging.CRITICAL)

    client = mqtt.Client()
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(broker_url, broker_port)
    client.subscribe(topic, qos=0)
    client.loop_forever()
