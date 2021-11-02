from minio import Minio
import logging
import paho.mqtt.client as mqtt
import io
import os
import base64
import json
import ProductImage
import sys

# Settig up the env variables, see index.md for further explanation
LOGGING_LEVEL = os.environ.get('LOGGING_LEVEL', 'DEBUG')
broker_url = os.environ['BROKER_URL']
broker_port = int(os.environ['BROKER_PORT'])
minio_url = os.environ['MINIO_URL']
minio_access_key = os.environ['MINIO_ACCESS_KEY']
minio_secret = os.environ['MINIO_SECRET_KEY']
minio_secure = bool(os.environ['MINIO_SECURE'])


bucket_name = os.environ['BUCKET_NAME']
topic = os.environ['TOPIC']

input_var = ""

IMAGE_FOLDER = "./images/"

# Connects to the mqtt client.
# If you want to be sure that the connection attempt was successful,
# then see the logs.
# This function will return the return code (rc) 0 if connected successfully.
# The following values are possible:
# 0 - Connection successful;
# 1 - Connection refused – incorrect protocol version;
# 2 - Connection refused – invalid client identifier;
# 3 - Connection refused – server unavailable;
# 4 - Connection refused – bad username or password;
# 5 - Connection refused – not authorised;
# 6-255 - Currently unused.


def on_connect(client, userdata, flags, rc):
    logging.info("Connected With Result Code " + str(rc))

# Message is an object and the payload property contains
# the message data which is binary data.


def on_message(client, userdata, message):      
    #Get Image from MQTT topic
    global result     
    try:
        result = ProductImage.product_image_from_dict(json.loads(message.payload))
    except:
        logging.warning("ProductImage failed to parse JSON payload. Please check your MQTT message format")
        return

    try:
        # Get image_id
        uid = result.image.image_id
        # Reading out image_bytes and decoding it from base64
        img_bytes = base64.b64decode(result.image.image_bytes, validate=True)
        
        # Write file to minio client
        minio_client.put_object(
                bucket_name=bucket_name, 
                object_name=uid + ".jpg", 
                data=io.BytesIO(img_bytes), 
                length=-1, 
                part_size=10*1024*1024,
                metadata={"timestamp_ms": result.timestamp_ms,
                          "image_id": result.image.image_id,
                          "image_height": result.image.image_height,
                          "image_width": result.image.image_width,
                          "image_channels": result.image.image_channels
                          }
                )
        
        logging.info("Successfully uploaded")
    except:
        logging.warning("Oops! %s occurred. Please check your MQTT topic again", sys.exc_info()[0])


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

    logging.debug(f"Broker URL: {broker_url}")
    logging.debug(f"Broker PORT: {broker_port}")
    logging.debug(f"MINIO URL: {minio_url}")
    logging.debug(f"Bucket NAME: {bucket_name}")
    
    # =============================================================================
    #     Connect to minio     
    # =============================================================================
    minio_client = Minio(
        minio_url,
        access_key=minio_access_key,
        secret_key=minio_secret,
        secure=minio_secure #Change to True if Minio is using https
    )
    
    # Connect or create to minio bucket
    found = minio_client.bucket_exists(bucket_name)

    if not found:
        minio_client.make_bucket(bucket_name)
    else:
        logging.info("Bucket already exists")

    # =============================================================================
    #     Call MQTT
    # =============================================================================
    global_client = mqtt.Client()
    global_client.on_connect = on_connect
    global_client.on_message = on_message
    global_client.username_pw_set("MQTT_TO_BLOB", password=None)
    global_client.connect(broker_url, broker_port)
    global_client.subscribe(topic, qos=0)
    global_client.loop_forever()
