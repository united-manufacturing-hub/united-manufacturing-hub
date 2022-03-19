"""
Classes to trigger the system.

Provided classes:
- MQTT-Trigger: triggering over MQTT broker
- ContinuousTrigger: triggering with a fixed provided cycle time
"""

# Import python in-built libraries
import datetime
import json
import sys
import time
import traceback
from typing import Union, Callable
import atexit
# Import libraries that had been installed with pip install
import paho.mqtt.client as mqtt
# import custom code
from utils import get_logger_from_env
from abc import ABC, abstractmethod
import cameras  # for typing only

logger = get_logger_from_env(application="cammeraconnect", name="trigger")
ERROR_TOLERANCE = 20  # number of successive errors after which the application is terminated
RETRY_DELAY = 0.1  # fraction of the cycle time after which to trigger a retry


class BaseTrigger(ABC):
    """
    base trigger with shared functionality, like error handling
    """

    def __init__(self):
        self.total_errors = 0
        self.errors_since_last_success = 0
        self.image_number = 0

    def count_error(self):
        """
        Counts errors occurred in total and since last successful message
        kills the process if excessive amount of errors occurred
        """
        self.errors_since_last_success += 1
        self.total_errors += 1
        logger.debug(f"error logged with counters total: {self.errors_since_last_success} "
                     f"successive: {self.total_errors}")
        if self.errors_since_last_success > ERROR_TOLERANCE:
            sys.exit(f"error tolerance exceeded with total errors {self.total_errors} "
                     f"and successive errors{self.errors_since_last_success}")

    def count_image(self):
        """
        resets the error count since last image
        Returns:

        """
        self.image_number += 1
        self.errors_since_last_success = 0

    def __del__(self):
        self.disconnect()

    @abstractmethod
    def disconnect(self):
        pass


class MqttTrigger(BaseTrigger):
    """
    This class provides an MQTT client instance to receive 
    trigger from the MQTT broker. Each instance is automatically 
    connected to the set MQTT broker. As soon as a a trigger is
    received, the callback function _on_message is executed.
    The method disconnect() disconnects from MQTT broker.

    Args of constructor:
        cam[Cognex/GenICam]:    A configured camera ready to
                                get an image. get_image() function
                                must be provided by object
        interface[string]:      Camera interface that is used
        acquisition_delay[float]:
                                Delay between trigger and 
                                acquisition
        mqtt_host[string]:      Hostname or IP address of the MQTT
                                broker
        mqtt_port[int]:         Network mqtt_port of the server
                                mqtt_host to connect to
        mqtt_topic[string]:     Topic on MQTT Broker where trigger
                                signal to save an image is send to
                                (e.g. "test/trigger/")

    Returns of constructor:
        A connected instance of MqttTrigger
    """

    def __init__(self, cam: Union[cameras.GenICam, cameras.DummyCamera, cameras.FasterGenICam], interface,
                 acquisition_delay, mqtt_host,
                 mqtt_port, mqtt_topic, retry_time=0) -> None:
        """
        Connect MQTT client.

        Args:
            see class description

        Returns:
            see class description
        """

        super().__init__()
        self.cam = cam
        self.interface = interface
        self.acquisition_delay = acquisition_delay
        self.mqtt_host = mqtt_host
        self.mqtt_port = mqtt_port
        self.mqtt_topic = mqtt_topic
        self.retry_time = retry_time
        # Connect to the Broker
        self.client = mqtt.Client()
        self.client.connect(self.mqtt_host, self.mqtt_port)
        # Start the loop to be always able to receive messages 
        #   from broker

        # Tests 10.04.2021
        self.client.on_subscribe = lambda client, userdata, mid, granted_qos: print(
            "Subscribed to topic: {}".format(self.mqtt_topic))

        self.client.loop_start()
        # Subscribe to the given mqtt_topic
        self.client.subscribe(self.mqtt_topic)
        logger.info("Subscribed for input to topic: " + str(self.mqtt_topic))
        # Call the _on_message when message is received from broker
        self.client.on_message = self._on_message
        self.image_number = 0
        atexit.register(self.__del__())

    # Is called always when a new message is received
    def __get_image_with_retry(self):
        """
        function to get image with a retry wraps the image class of the camera
        and adds retry logic

        Returns:

        """
        # loops to get an image after the retry time is over in case of an acquisition failure, only works if retry time is configured
        while True:
            try:
                self.cam.get_image()
                self.count_image()
                break
            except Exception as _e:
                self.count_error()
                if self.retry_time == 0:
                    logger.error(f"Failed to get image at:{datetime.datetime.now(tz=datetime.timezone.utc)} "
                                 f"with {_e} "
                                 f"no retry time configured, aborting")
                    sys.exit(f"could not gather triggered mage with {traceback.format_exc()}")
                logger.error(f"Failed to get image at:{datetime.datetime.now(tz=datetime.timezone.utc)} "
                             f"with {_e} "
                             f", retrying in {self.retry_time} ")
                time.sleep(self.retry_time)

    def _on_message(self, client, userdata, msg) -> None:
        """
        Callback function for MQTT on_message. 
        Message must be a encoded json!

        Gets a new image with the cam object.

        Args:
            client:         client instance for this callback
            userdata:       private user data as set in Client() 
                            or user_data_set()
            msg:        an instance of MQTTMessage. This is a
                            class with members topic, payload, 
                            qos, retain.

        Returns:
            None     
        """

        # noinspection PyBroadException
        try:
            message = json.loads(msg.payload)
            logger.info("Image acquisition trigger received")

            # If no acquisition delay skip the following
            # if acquisition dealy is configured it will wait either until the configured delay
            # this delay is either relative to the time the messages was received
            # or when a timestamp is given iun the trigger message, that timestamp is used as reference
            if self.acquisition_delay > 0.0:
                # Check if timestamp in ms is provided in message. Then
                # use this timestamp instead.
                if 'timestamp_ms' in message:
                    timestamp_ms = int(message['timestamp_ms'])
                else:
                    timestamp_ms = time.time() * 1000  # sets acquisition to start after acquisition delay
                    # if no timestamp is provided
                time_to_get_image: float = timestamp_ms + round(self.acquisition_delay * 1000)  # unix time in ms

                if time_to_get_image < round(time.time() * 1000):
                    logger.critical("Environment Error: ACQUISITION_DELAY to short ||| Set acquisition delay is "
                                    "shorter than the processing time.")
                    sys.exit(-1)

                time_to_wait = (
                                       time_to_get_image / 1000) - time.time()  # converts ms to delta s until image needs to be taken
                try:
                    logger.debug(f"sleeping for {time_to_wait} to capture image")
                    time.sleep(time_to_wait)
                except ValueError:
                    logger.error(f"could not wait for image acquisition with delay: {time_to_wait}")
                    self.count_error()

            # Get an image
            logger.info("Get an image.")

            # loop to retry gathering an image if one acquisition fails
            self.__get_image_with_retry()

        except Exception as _e:  # wildcard to catch interesting side effect of paho mqtt's threading behaviour +
            # harvesters instability with certain producer files, which sometimes lead to zombie threads blocking the
            # acquisition indefinitely
            self.count_error()
            logger.error(f"failed to process message: %s with %s \n%s", msg, _e, traceback.format_exc())

    def disconnect(self) -> None:
        """
        Disconnects from MQTT broker.

        Args:
 
        Returns:
            None
        """
        self.client.loop_stop()
        self.client.disconnect()
        self.cam.disconnect()


class ContinuousTrigger(BaseTrigger):
    """
    This class provides a continuous trigger that acquires images
    with a fixed cycle time. All functionalities are provided
    in the constructor. No methods. Object already stays in a 
    while-loop so that the process never jumps out of this 
    instance.

    Args of constructor:
        cam[Cognex/GenICam]:    A configurated camera ready to 
                                get an image. get_image() function
                                must be provided by object
        interface[string]:      Camera interface that is used
        cycle_time[float]:      Time between each image acqui-
                                sition in seconds

    Returns of constructor:
        Continuous triggering instance in which the process will
        stay forever.
    """

    def __init__(self, cam: Union[cameras.GenICam, cameras.DummyCamera], interface, cycle_time) -> None:
        """
        While-loop with time measuring to have always the same 
        cycle time.

        Args:
            see class description

        Returns:
            see class description
        """

        super().__init__()
        self.cam = cam
        self.interface = interface
        self.cycle_time = cycle_time
        self.total_errors = 0
        self.errors_since_last_success = 0
        # Start the loop to take an image according to cycle time

    def __loop_cycle(self, image_function_: Callable, cycle_time):
        """
        logic for a single loop cycle
        Args:
            image_function_ ():
            cycle_time ():

        Returns:

        """
        # Get actual time and save it as start time
        timer_start = time.time()
        # get image, retry if it fails
        while cycle_time > abs(time.time() - timer_start):
            try:
                image_function_()
                self.count_image()
                break
            except Exception as _e:
                self.count_error()
                ttw = cycle_time * RETRY_DELAY  # verbosity is more important than precision in this case
                logger.error(f"Failed to get image at:{datetime.datetime.now(tz=datetime.timezone.utc)} "
                             f"with {_e} "
                             f", retrying in {ttw} ")
                self.count_error()
                time.sleep(ttw)

        # Get actual time and subtract the start time from
        # it to get the time which the current loop needed
        # to run the code
        logger.debug(f"cycle time: {cycle_time}")
        loop_time = time.time() - timer_start
        # If the processing time is longer than the cycle  counts as error
        if loop_time > cycle_time:
            logger.critical(
                "Environment Error: CYCLE_TIME to short ||| Set cycle time is "
                "shorter than the processing time for each image.")
            logger.error(f"cycle time: {self.cycle_time} loop took {loop_time}")
            self.count_error()

        else:
            # Sleep for difference of cycle time minus loop
            # time to have a constant cycle time
            delay = self.cycle_time - loop_time
            self.errors_since_last_success = 0
            logger.debug(f"Delay {delay} s to reach constant cycle time.")
            time.sleep(delay)

    def start_loop(self, duration=10, forever=True):
        """
        starts the loop of the trigger to get images.
        when forever = true it will continue forever, if forever is false it will record for a set duration
        Args:
            forever ():
            duration ():

        Returns:

        """

        image_function = self.cam.get_image
        if forever:
            while True:
                self.__loop_cycle(image_function, self.cycle_time)
        else:
            end_time = time.time() + duration
            while time.time() < end_time:
                self.__loop_cycle(image_function, self.cycle_time)

    def disconnect(self):
        self.cam.disconnect()


class FPSTrigger(BaseTrigger):

    def __init__(self, cam, target_fps, no_inference=False) -> None:
        """

        This class provides a continuous trigger that acquires images at a given framerate
        only support faster Genicam

        Args:
            see class description

        Returns:
            see class description
        """

        if not issubclass(type(cam), cameras.FasterGenICam):
            logger.warning("camera of class %s may not be able to deliver the desired framerate",
                           type(cam))
        super().__init__()

        self.cam = cam
        self.target_fps = target_fps
        self.no_inference = no_inference

    def __loop_cycle(self, image_function: Callable, cycle_time_):
        """
        logic for a single loop cycle
        Args:
            image_function ():
            cycle_time_ ():

        Returns:

        """
        cycle_end = time.time() + cycle_time_
        try:
            image_function()
            self.count_image()
        except Exception as _e:  # hard error during acquisition
            logger.error("failed to get image with unhandled exception with %s, \n%s", _e, traceback.format_exc())
            self.count_error()
        try:
            time_to_sleep = cycle_end - time.time()
            time.sleep(time_to_sleep)
        except ValueError:  # timeout of loop cycle, frame drops are handled by the camera itself
            pass

    def start_loop(self, duration=10, forever=True):

        # useful if multiple get image functions are implemented, this may be a desired feature in the future

        cycle_time = 1 / self.target_fps

        image_function = self.cam.get_image

        # Start the loop to take an image according to cycle time
        if forever:
            while True:
                self.__loop_cycle(image_function, cycle_time)
        else:
            end_time = time.time() + duration
            while time.time() < end_time:
                self.__loop_cycle(image_function, cycle_time)

    def disconnect(self):
        self.cam.disconnect()
