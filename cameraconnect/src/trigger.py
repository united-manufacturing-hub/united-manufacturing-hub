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
import logging

# Import libraries that had been installed with pip install
import paho.mqtt.client as mqtt

ERROR_TOLERANCE = 20  # number of successive errors after which the application is terminated
RETRY_DELAY = 0.1  # fraction of the cycle time after which to trigger a retry


class MqttTrigger:
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

    def __init__(self, cam, interface, acquisition_delay, mqtt_host, mqtt_port, mqtt_topic) -> None:
        """
        Connect MQTT client.

        Args:
            see class description

        Returns:
            see class description
        """

        self.cam = cam
        self.interface = interface
        self.acquisition_delay = acquisition_delay
        self.mqtt_host = mqtt_host
        self.mqtt_port = mqtt_port
        self.mqtt_topic = mqtt_topic

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
        print("Subscribed for input to topic: " + str(self.mqtt_topic))
        # Call the _on_message when message is received from broker
        self.client.on_message = self._on_message

    # Is called always when a new message is received
    def _on_message(self, client, userdata, msg) -> None:
        """
        Callback function for MQTT on_message. 
        Message must be a encoded json!

        Gets a new image with the cam object.

        Args:
            client:         client instance for this callback
            userdata:       private user data as set in Client() 
                            or user_data_set()
            message:        an instance of MQTTMessage. This is a
                            class with members topic, payload, 
                            qos, retain.

        Returns:
            None     
        """
        # If no acquisition delay skip the following
        if self.acquisition_delay > 0.0:
            # Get timestamp of time  when trigger was received. 
            #   Measured in ms since epoch. Epoch is defined as 
            #   January 1, 1970, 00:00:00 (UTC)
            timestamp_ms = int(round(time.time() * 1000))

        # Deserialize Json
        message = json.loads(msg.payload)
        print("Image acquisition trigger received")

        # If no acquisition delay skip the following
        if self.acquisition_delay > 0.0:
            # Check if timestamp in ms is provided in message. Then
            #   use this timestamp instead.
            if 'timestamp_ms' in message:
                timestamp_ms = int(message['timestamp_ms'])
            time_to_get_image: float = timestamp_ms + round(self.acquisition_delay * 1000)  # unix time in ms

        # If no acquisition delay skip the following
        if self.acquisition_delay > 0.0:

            if time_to_get_image < round(time.time() * 1000):
                sys.exit(
                    "Environment Error: ACQUISITION_DELAY to short ||| Set acquisition delay is shorter than the processing time.")
            time_to_wait = time_to_get_image / 1000 - time.time()
            if 60 * 60 > time_to_wait > 0:  # in case of transformation error does not freeze the process for more
                # than 1 hour
                logging.debug(f"sleeping for {time_to_wait} to capture image")
                time.sleep(time_to_wait)
            else:
                logging.error(f"could not wait for image acquisition with delay: {time_to_wait}")

        # Get an image
        print("Get an image.")
        self.cam.get_image()

    def disconnect(self) -> None:
        """
        Disconnects from MQTT broker.

        Args:
            None
 
        Returns:
            None
        """
        self.client.loop_stop()
        self.client.disconnect()


class ContinuousTrigger:
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

    def __count_error(self):
        """
        Counts errors occurred in total and since last successful message
        kills the process if excessive amount of errors occurred
        """
        self.errors_since_last_success += 1
        self.total_errors += 1
        logging.debug(f"error logged with counters total: {self.errors_since_last_success} "
                      f"successive: {self.total_errors}")
        if self.errors_since_last_success > ERROR_TOLERANCE:
            sys.exit(f"error tolerance exceeded with total errors {self.total_errors} "
                     f"and successive errors{self.errors_since_last_success}")

    def __init__(self, cam, interface, cycle_time) -> None:
        """
        While-loop with time measuring to have always the same 
        cycle time.

        Args:
            see class description

        Returns:
            see class description
        """
        self.cam = cam
        self.interface = interface
        self.cycle_time = cycle_time
        self.total_errors = 0
        self.errors_since_last_success = 0
        # Start the loop to take an image according to cycle time
        while True:
            # Get actual time and save it as start time
            timer_start = time.time()
            while cycle_time > abs(time.time() - timer_start):
                try:
                    self.cam.get_image()
                    break
                except Exception as _e:
                    ttw = cycle_time * RETRY_DELAY
                    logging.critical(f"Failed to get image at:{datetime.datetime.now(tz=datetime.timezone.utc)} "
                                     f"with {_e} "
                                     f", retrying in {ttw} ")
                    self.__count_error()
                    time.sleep(ttw)

            # Get actual time and subtract the start time from
            #   it to get the time which the current loop needed 
            #   to run the code
            logging.debug(f"cycle time: {cycle_time}")
            loop_time = time.time() - timer_start
            # If the processing time is longer than the cycle 
            #   time throw error
            if loop_time > self.cycle_time:
                logging.critical(
                    "Environment Error: CYCLE_TIME to short ||| Set cycle time is "
                    "shorter than the processing time for each image.")
                logging.error(f"cycle time: {cycle_time} loop took {loop_time}")
                self.__count_error()

            else:
                # Sleep for difference of cycle time minus loop 
                # time to have a constant cycle time
                delay = self.cycle_time - loop_time
                self.errors_since_last_success = 0
                logging.debug(f"Delay {delay} s to reach constant cycle time.")
                time.sleep(delay)
