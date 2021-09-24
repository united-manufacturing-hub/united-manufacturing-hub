"""
Main script that is executed insight the docker container.
It imports the required libraries and self-written modules
as well as the environment variables. The defintion of the 
environment variables can be found in the env-file. 

Depending on the settings of the environment variables,
objects of classes are intantiated which provides all 
necessary connections and functionalities. To find out 
more about the classes take a look into the imported 
modules. If there is not a while-loop already provided by
the instance, a while-loop is used to keep everything 
running until the user stops the container.
"""

# Import python in-built libraries
import glob
import time
import os
import sys
import logging

# Import self-written modules
from cameras import GenICam
from cameras import DummyCamera
from trigger import MqttTrigger, ContinuousTrigger

IMAGE_PATH = os.environ.get('IMAGE_PATH', None)

### LOAD OVERALL SETTINGS
## MQTT SETTINGS
MQTT_HOST = os.environ.get('MQTT_HOST')
MQTT_PORT = int(os.environ.get('MQTT_PORT', 1883))

## TRIGGER & PROCESS SETTINGS
TRIGGER = os.environ.get('TRIGGER')
ACQUISITION_DELAY = float(os.environ.get('ACQUISITION_DELAY', 0.0))
CYCLE_TIME = float(os.environ.get('CYCLE_TIME', 10.0))

## CAMERA SETTINGS
CAMERA_INTERFACE = os.environ.get('CAMERA_INTERFACE')
MAC_ADDRESS = os.environ.get('MAC_ADDRESS', '')
TRANSMITTER_ID = os.environ.get('CUBE_TRANSMITTERID', '')

MQTT_TOPIC_TRIGGER = "ia/trigger/" + TRANSMITTER_ID + "/" + MAC_ADDRESS  # todo add variable for first part of topic
MQTT_TOPIC_IMAGE = "ia/rawImage/" + TRANSMITTER_ID + "/" + MAC_ADDRESS  # todo add variable for first part of topic

# GenICam settings
DEFAULT_GENTL_PRODUCER_PATH = os.environ.get('DEFAULT_GENTL_PRODUCER_PATH', '/app/assets/producer_files')
USER_SET_SELECTOR = os.environ.get('USER_SET_SELECTOR', 'Default')
IMAGE_WIDTH = int(os.environ.get('IMAGE_WIDTH', 800))
IMAGE_HEIGHT = int(os.environ.get('IMAGE_HEIGHT', 800))
PIXEL_FORMAT = os.environ.get('PIXEL_FORMAT', 'Mono8')
IMAGE_CHANNELS = os.environ.get('IMAGE_CHANNELS', 'None')
EXPOSURE_TIME = os.environ.get('EXPOSURE_TIME', 'None')

EXPOSURE_AUTO = os.environ.get('EXPOSURE_AUTO', 'Off')
if EXPOSURE_AUTO.upper() == "OFF" or EXPOSURE_AUTO.upper() == "NONE":
    EXPOSURE_AUTO = None
if EXPOSURE_TIME.upper() == "OFF" or EXPOSURE_TIME.upper() == "NONE":
    EXPOSURE_TIME = None
GAIN_AUTO = os.environ.get('GAIN_AUTO', 'Off')
BALANCE_WHITE_AUTO = os.environ.get('BALANCE_WHITE_AUTO', 'Off')
LOGGING_LEVEL = os.environ.get('LOGGING_LEVEL', 'INFO')
LOG_FILE = os.environ.get("LOG_FILE", None)  # todo undocumented
if IMAGE_CHANNELS != 'None':
    IMAGE_CHANNELS = int(IMAGE_CHANNELS)


### End of loading settings ###
if __name__ == "__main__":

    if LOGGING_LEVEL == "DEBUG":
        logging.basicConfig(level=logging.DEBUG, filename=LOG_FILE,filemode="w")
    elif LOGGING_LEVEL == "INFO":
        logging.basicConfig(level=logging.INFO, filename=LOG_FILE,filemode="w")
    elif LOGGING_LEVEL == "WARNING":
        logging.basicConfig(level=logging.WARNING, filename=LOG_FILE)
    elif LOGGING_LEVEL == "ERROR":
        logging.basicConfig(level=logging.ERROR, filename=LOG_FILE)
    elif LOGGING_LEVEL == "CRITICAL":
        logging.basicConfig(level=logging.CRITICAL, filename=LOG_FILE)

    if EXPOSURE_TIME != 'None':
        try:
            EXPOSURE_TIME = float(EXPOSURE_TIME)
        except TypeError:
            exposure_default = 15000.0
            logging.warning(f"exposure not valid using default of:  {exposure_default}")
            EXPOSURE_TIME = exposure_default

    logging.debug("Exposure time: " + str(EXPOSURE_TIME))
    logging.debug("Image channels: " + str(IMAGE_CHANNELS))
    logging.debug("Set image width: " + str(IMAGE_WIDTH))
    logging.debug("Set image height: " + str(IMAGE_HEIGHT))

    # detect available cti files as camera producers
    cti_file_list = []
    for name in glob.glob(str(DEFAULT_GENTL_PRODUCER_PATH) + '/**/*.cti', recursive=True):
        cti_file_list.append(str(name))

    # if no cti files are found, log error and exit program
    if len(cti_file_list) == 0:
        logging.error("No producer file discovered")
        exit(1)

    # Check selected camera interface
    if CAMERA_INTERFACE == "DummyCamera":
        cam = DummyCamera(MQTT_HOST, MQTT_PORT, MQTT_TOPIC_IMAGE, 0, image_storage_path=IMAGE_PATH)
    elif CAMERA_INTERFACE == "GenICam":
        logging.debug("looking for GenICam")
        cam = GenICam(MQTT_HOST, MQTT_PORT, MQTT_TOPIC_IMAGE, MAC_ADDRESS, cti_file_list, image_width=IMAGE_WIDTH,
                      image_height=IMAGE_HEIGHT, pixel_format=PIXEL_FORMAT, image_storage_path=IMAGE_PATH,
                      exposure_time=EXPOSURE_TIME, exposure_auto=EXPOSURE_AUTO)

    else:
        # Stop system, not possible to run with this settings
        sys.exit(
            "Environment Error: CAMERA_INTERFACE not supported ||| Make sure to set a value that is allowed according to the specified possible values for this environment variable and make sure the spelling is correct.")

    # Check trigger type and use appropriate instance of the
    #   trigger classes
    logging.debug(f"looking for trigger type {TRIGGER}")
    if TRIGGER == "Continuous":
        # Never jumps out of the processes of the instance
        ContinuousTrigger(cam, CAMERA_INTERFACE, CYCLE_TIME)
    elif TRIGGER == "MQTT":
        # Starts an asynchroneous process for working with 
        #   the received mqtt data
        trigger = MqttTrigger(cam, CAMERA_INTERFACE, ACQUISITION_DELAY, MQTT_HOST, MQTT_PORT, MQTT_TOPIC_TRIGGER)

        # Run forever to stay connected 
        while True:
            # Avoid overloading the CPU
            time.sleep(10)
            logging.debug("Still running.")
    else:
        # Stop system, not possible to run with this setting
        sys.exit(
            "Environment Error: TRIGGER not supported ||| Make sure to set a value that is allowed according to the specified possible values for this environment variable and make sure the spelling is correct.")
