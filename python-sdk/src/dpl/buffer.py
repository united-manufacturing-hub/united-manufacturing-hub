# -- coding: utf-8 --
"""
Created on 2020-04-03
@author: Ricardo Vega Ayora
"""

# STD imports
import enum
import json
import os
import time
import traceback

# 3rd party imports
import paho.mqtt.client as mqtt
import pandas as pd

# local imports

# Constants

# Retention types - points or seconds
class BufferRetentionType(enum.Enum):
    """Set of retention types.
    """
    SECONDS = 1
    POINTS = 2

class MqttBuffer(object):
    """Subscribes to an MQTT topic and buffers the messages it receives.


    """

    ### Temporary(?) logging methods
    # May the Python gods forgive me for not following the PEP8 79-column rule
    # TODO Change all print messages for exceptions.
    _timestring = lambda s: str(time.ctime(time.time()))
    _msgstring = lambda s, level, module, msg: "{} [{}] DPL {}: {}".format(s._timestring(), level, module, msg)
    _print_info = lambda s, module, msg: print(s._msgstring('INFO', module, msg))
    _print_error = lambda s, module, msg: print(s._msgstring('ERROR', module, msg))

    ### Instantiate client
    def __init__(self, buffer_size, **kwargs):
        """Constructor. DO NOT CALL DIRECTLY! Use the
        MqttBuffer.create_point_buffer and MqttBuffer.create_timeseries
        methods instead.
        """
        # Default values
        self._buffer_size = buffer_size
        
        # All arguments are passed by the creation methods, with user defined
        # or default values, so these can be passed directly to the class's
        # properties vie **kwargs
        self._buffer_columns = kwargs['buffer_columns']
        self._buffer_type = kwargs['buffer_retention_type']
        self._mqtt_broker_url = kwargs['broker_url']
        self._mqtt_port = kwargs['broker_port']
        self._mqtt_sub_topic = kwargs['sub_topic']
        self._mqtt_pub_topic = kwargs['pub_topic']
        self._mqtt_client_id = kwargs['client_id']

        # Initialise buffer stuff
        self._buffer_init()
        
        # Initialise MQTT sutff if the URL and the port were given
        self._mqtt_init()

        # Set the parsing and the processing functions
        self.mqtt_parse_function = None
        self.processing_function = None             

    ### Instantiation and access methods
    @property
    def buffer(self):
        return self._buffer

    @buffer.setter
    def buffer(self, bf):
        self._buffer = bf

    @classmethod
    def create_point_buffer(cls, buffer_size, buffer_columns=['value'],
                            broker_url="", broker_port=8883, sub_topic="",
                            pub_topic="", client_id=""):
        """Creates a datapoint buffer
        Arguments:
        buffer_size     Depending on the selected buffer_retention_type,
        [int]           this argument defines the number of datapoints to
                        be stored or the number of seconds to hold data for
        buffer_columns  Defines how many columns the buffer should have and
        [string array]  what their names should be. Defaults to the single
                        column ['value'].
        broker_url      MQTT broker's URL. The MQTT connection will not be
        [string]        established without this argument. Defaults to an
                        empty string.
        broker_port     MQTT broker's port. Defaults to 8883.
        [int]
        sub_topic       MQTT topic to subscribe to. The MQTT connection will
        [string]        not be established without this argument. Defaults to
                        an empty string.
        pub_topic       MQTT topic to publish to. The MQTT connection will
        [string]        not be established without this argument. Defaults to
                        an empty string.
        client_id       Client ID for the MQTT connection. Defaults to an
        [string]        empty string.
        """
        return cls(buffer_size=buffer_size,
                    buffer_columns=buffer_columns,
                    buffer_retention_type=BufferRetentionType.POINTS,
                    broker_url=broker_url,
                    broker_port=broker_port,
                    sub_topic=sub_topic,
                    pub_topic=pub_topic,
                    client_id=client_id
        )

    @classmethod
    def create_timeseries(cls, buffer_size, buffer_columns=['value'],
                        broker_url="", broker_port=8883, sub_topic="",
                        pub_topic="", client_id=""):
        """Creates a timeseries buffer
        Arguments:
        buffer_size     Depending on the selected buffer_retention_type,
        [int]           this argument defines the number of datapoints to
                        be stored or the number of seconds to hold data for
        buffer_columns  Defines how many columns the buffer should have and
        [string array]  what their names should be. Defaults to the single
                        column ['value'].
        broker_url      MQTT broker's URL. The MQTT connection will not be
        [string]        established without this argument. Defaults to an
                        empty string.
        broker_port     MQTT broker's port. Defaults to 8883.
        [int]
        sub_topic       MQTT topic to subscribe to. The MQTT connection will
        [string]        not be established without this argument. Defaults to
                        an empty string.
        pub_topic       MQTT topic to publish to. The MQTT connection will
        [string]        not be established without this argument. Defaults to
                        an empty string.
        client_id       Client ID for the MQTT connection. Defaults to an
        [string]        empty string.
        """
        return cls(buffer_size=buffer_size,
                    buffer_columns=buffer_columns,
                    buffer_retention_type=BufferRetentionType.SECONDS,
                    broker_url=broker_url,
                    broker_port=broker_port,
                    sub_topic=sub_topic,
                    pub_topic=pub_topic,
                    client_id=client_id
        )

    def _buffer_init(self):
        # TODO check for type errors here

        if (self._buffer_type == BufferRetentionType.SECONDS):
            self._buffer_size = pd.Timedelta(seconds=self._buffer_size)

        # Set up columns
        df_columns = self._buffer_columns
        if 'timestamp' not in self._buffer_columns:
            df_columns = ['timestamp'] + self._buffer_columns

        # Variable used to check if the buffer is full
        self._buffer_num_columns = len(df_columns)
        self._buffer = pd.DataFrame(columns=df_columns)

    def _buffer_is_full(self):
        # The buffer should hold at least one point for this to work.
        # Check buffer's actual length. The size property of a data frame
        # returns the number of rows times the number of columns and since
        # we are only interested in the rows, we divide by the number of
        # columns.
        current_buffer_length = self._buffer.size/self._buffer_num_columns
        if (current_buffer_length < 1):
            return False
        # If the retention type is seconds, check the time difference between
        # the first and last timestamps
        elif (self._buffer_type == BufferRetentionType.SECONDS):
            first_timestamp = self._buffer.iloc[0]['timestamp']
            last_timestamp = self._buffer.iloc[-1]['timestamp']

            # If the newest timestamp is ahead (oldest + buffer_size), then the
            # buffer is full. With Pandas timestamps, past < future = true
            return last_timestamp > (first_timestamp + self._buffer_size)

        # If the retention type is data points, check the length of the buffer
        elif (self._buffer_type == BufferRetentionType.POINTS):  
            # Return true if the current size is larger than the one
            # defined when instantiated.
            if (current_buffer_length >= self._buffer_size):
                return True
            return False
        
        # Something went very wrong
        raise Exception('Wrong BufferRetentionType.')

    def _mqtt_init(self):
        # Emtpy strings return false
        if (self._mqtt_broker_url
            and self._mqtt_sub_topic
            and self._mqtt_pub_topic):

            self._mqtt_client = mqtt.Client(client_id=self._mqtt_client_id)
            self._mqtt_client.on_connect = self._mqtt_on_connect
            self._mqtt_client.on_subscribe = self._mqtt_on_subscribe
            self._mqtt_client.on_message = self._mqtt_on_message

            # Connect non-blocking
            self._print_info('MQTT', 'Connecting to broker {}:{}'.format(
                self._mqtt_broker_url, self._mqtt_port
            ))
            self._mqtt_client.connect_async(self._mqtt_broker_url,
                                        port=self._mqtt_port,
                                        keepalive=65535)
            self._mqtt_client.loop_start()

    ### MQTT functions
    def _mqtt_on_connect(self, client, obj, flags, rc):
        # Report a successful connection to the broker
        self._print_info('MQTT', 'Connected to broker.')
        
        # Subscribe to the topic only after the connection is
        # succesfully established.
        self._print_info('MQTT', 'Subscribing to topic {}.'.format(self._mqtt_sub_topic))
        self._mqtt_client.subscribe(self._mqtt_sub_topic)

    def _mqtt_on_subscribe(self, client, obj, mid, gqos):
        # Report a successful subscription and start the buffering
        self._print_info('MQTT', 'Subscribed to topic.')
        self._print_info('MQTT', 'Starting buffering.')

    def _mqtt_on_message(self, client, userdata, message):
        # Check the existing data points to shift the dataset accordingly
        # to fit the new data point. The rule is FIFO.

        # Parse the mqtt message
        # The function should return an dictionary with the defined
        # columns as keys
        try:
            if (self.mqtt_parse_function):
                decoded_message = message.payload.decode("utf-8")
                parsed_point = self.mqtt_parse_function(decoded_message)

                # Allow parse functions to return none for points that should not
                # be buffered.
                if parsed_point == None:
                    return
                else:
                    datapoint = parsed_point

            else:
                # If there is no parsing function, 
                # no data can be stored in the buffer
                return 
        except Exception as e:
                self._print_error('BUFFER',
                                'Error in parsing function: {}'.format(e))
                track = traceback.format_exc()
                print(track)
                raise e

        try:
            # Create a new timestamp if none was added by the user
            if 'timestamp' not in datapoint:
                datapoint["timestamp"] = pd.Timestamp.now()
 
            # Add the new values to the buffer
            new_point = pd.DataFrame.from_records([datapoint])

            # If the buffer is full, remove the oldest row
            if (self._buffer_is_full()):
                self._buffer = self._buffer.drop(self._buffer.index[0])
            
            # Insert the new point
            self._buffer = self._buffer.append(new_point, ignore_index=True)

        except Exception as e:
                self._print_error('BUFFER',
                                'Error in adding datapoint to buffer: {}'.format(e))
                track = traceback.format_exc()
                print(track)
                raise e

        # Process data in buffer and publish it
        if (self.processing_function):
            try:
                # Process functions should return None if no message is to be published
                payload = self.processing_function(self._buffer)

                # Publish onlly if the function returns something
                if (payload):
                    json_payload = json.dumps(payload)
                    self._mqtt_client.publish(topic=self._mqtt_pub_topic,
                                            payload=json_payload, qos=1)
            except Exception as e:
                self._print_error('BUFFER',
                                'Error in processing function: {}'.format(e))
                track = traceback.format_exc()
                print(track)
                raise e