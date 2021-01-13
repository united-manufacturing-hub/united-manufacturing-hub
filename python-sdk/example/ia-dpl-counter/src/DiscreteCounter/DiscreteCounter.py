# -- coding: utf-8 --
"""
Created on 2020-04-03
@author: Ricardo Vega Ayora, Jeremy Theocharis
"""
# STD imports
import os
import time 

# 3rd party imports
import json
import pandas as pd

# local imports
from dpl import buffer as bf

# TODO Document
class DiscreteCounter():
    # Logging functions
    _timestring = lambda s: str(time.ctime(time.time()))
    _msgstring = lambda s, level, module, msg: "{} [{}] DiscreteCounter {}: {}".format(s._timestring(), level, module, msg)
    _print_info = lambda s, module, msg: print(s._msgstring('INFO', module, msg))
    _print_warn = lambda s, module, msg: print(s._msgstring('WARN', module, msg))
    _print_error = lambda s, module, msg: print(s._msgstring('ERROR', module, msg))

    def __init__(self, broker_url="",
                        broker_port="",
                        client_id="",

                        customer_id="",
                        location="",
                        machine_id="",
                        transmitter_id="",

                        sub_topic="",
                        pub_topic="",
                        
                        distance_threshold="",
                        datapoint_name="",
                        mode=""
                        ):

        self.point_buffer = bf.MqttBuffer.create_point_buffer(
            buffer_size=2,
            broker_url=broker_url,
            broker_port=broker_port,
            sub_topic=sub_topic,
            pub_topic=pub_topic,
            client_id=client_id
        )

        # General variables
        self.customer_id=customer_id
        self.location=location
        self.machine_id=machine_id
        self.transmitter_id=transmitter_id

        #Trigger Variable
        self.mode=mode

        # Pass parsing function to the internal buffer
        self.point_buffer.mqtt_parse_function = self.parseLightBarrier

        # Pass counting function to the internal buffer
        self.point_buffer.processing_function  = self.count_up

        # Couting variables
        self.distance_threshold = float(distance_threshold)
        self.key_string = datapoint_name

    def parseLightBarrier(self, mqtt_message):
        # Extract value from message, which is passed as a string
        payload = json.loads(mqtt_message)
        
        # Check if the key we are insterested in is in the payload
        if self.key_string in payload:
            reading = float(payload[self.key_string])
            # Compare reading to threshold
            if (self.mode == 1 and reading < self.distance_threshold):
                datapoint = {
                    'count': 1,
                    'timestamp': pd.to_datetime(payload['timestamp_ms'], unit='ms')
                }
                return datapoint
            elif (self.mode == 2 and reading > self.distance_threshold):
                datapoint = {
                    'count': 1,
                    'timestamp': pd.to_datetime(payload['timestamp_ms'], unit='ms')
                }
            else:
                datapoint = {
                'count': 0,
                'timestamp': pd.to_datetime(payload['timestamp_ms'], unit='ms')
            }
            return datapoint
        else:
            self._print_warn('parseLightBarrier','No {} value in message.'.format(self.key_string))
            return None

    def count_up(self, buffer):
        """Counter. Counts rising edges from 0 to 1.
        """
        # Check previous state in the first row
        previous_state = buffer.iloc[0]['count']

        # Check current state in the last row
        current_state = buffer.iloc[-1]['count']

        # using the original timestamp
        timestamp_ms = int(buffer.iloc[-1]['timestamp'].to_datetime64().astype('uint64')/1e6)

        # If the states are the same, no item or pulse has been identified.
        # If not, count only on rising edges
        if (current_state > previous_state):
            # Counting only on rising edge in this version
            payload = {
                "serial_number": self.customer_id + "-" + self.location + "-" + self.machine_id,
                "timestamp_ms": timestamp_ms,
                "count": 1
            }
            self._print_info("count_up", "Count detected")

            return payload # currently useless in this DPL version
        
        # Return None otherwise
        return None