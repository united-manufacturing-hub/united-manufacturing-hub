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
broker_port = int(os.environ['BROKER_PORT'])
minio_url = os.environ['MINIO_URL']
minio_access_key = os.environ['MINIO_ACCESS_KEY']
minio_secret = os.environ['MINIO_SECRET_KEY']
bucket_name = os.environ['BUCKET_NAME']
topic = os.environ['TOPIC']
input_var = ""
timezone = pytz.timezone('Europe/Vienna')
logging.basicConfig(level=logging.DEBUG)

# Create and configure logger
logging.basicConfig(filename="newfile.log",
                    format='%(asctime)s %(message)s',
                    filemode='w')
# Creating an object
logger = logging.getLogger()
#Setting the threshold of logger to DEBUG
logger.setLevel(logging.DEBUG)

def on_connect(client, userdata, flags, rc):
    logging.info("Connected With Result Code " + str(rc))

def on_message(client, userdata, message):
    input_var = json.loads(message.payload)
    #changed from timestamp to imageID. If the file should be named by the timestamp then use this line instead of the uid.
    #timestamp = datetime.datetime.fromtimestamp(int(input_var["timestamp_ms"])/1000.0, timezone).strftime("%Y%m%d_%H-%M-%S-%f")
    im = input_var["image"]
    uid = input_var["imageUID"]
    im_bytes = base64.b64decode(im["image_bytes"])
    im_arr = np.frombuffer(im_bytes, dtype=np.uint8)
    img = cv2.imdecode(im_arr, flags=cv2.IMREAD_COLOR)
    img_saver = cv2.imwrite("./images/"+uid+".jpg", img)
    if img_saver==True:
        logger.info("saved")
    else:
        logger.debug("failed to save image to cache")
    client = Minio(
        minio_url,
        access_key=minio_access_key,
        secret_key=minio_secret
    )
    found = client.bucket_exists(bucket_name)
    if not found:
        client.make_bucket(bucket_name)
    else:
        logger.info("Bucket bucket_name already exists")
    client.fput_object(
        bucket_name, uid +".jpg", "./images/" + uid + ".jpg"
    )
    logger.info("Successfully uploaded")

    if os.path.exists("./images/" + uid + ".jpg"):
        os.remove("./images/" + uid + ".jpg")
        logger.info("file has been deleted")
    else:
        logger.info("The file does not exist")

if __name__ == "__main__":
    client = mqtt.Client()
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(broker_url, broker_port)
    client.subscribe(topic, qos=0)
    client.loop_forever()
