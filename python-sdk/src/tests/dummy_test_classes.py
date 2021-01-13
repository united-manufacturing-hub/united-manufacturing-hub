import time
from dpl.buffer import MqttBuffer as Buffer

# Dummy classes
class Payload(object):
    def __init__(self, payload):
        self.payload = payload

    def decode(self, encoding):
        return str(self.payload)

class Message(object):
    def __init__(self, payload):
        self.payload = Payload(payload=payload)

def fill_point_buffer(buffer, buffer_cap):
    for payload in range(0, buffer_cap):
        message = Message(payload=payload)
        buffer._mqtt_on_message(client=None, userdata=None, message=message)
    return buffer

def fill_timeseries(buffer, buffer_cap):
    # Add one to make sure the correct number of elements are
    # added in the specified time
    for payload in range(0, buffer_cap+1):
        message = Message(payload=payload)
        buffer._mqtt_on_message(client=None, userdata=None, message=message)
        time.sleep(1)
    return buffer

def create_fill_point_buffer(bf_size, bf_fill_level, parse_fn=False):
    # Create
    bf = Buffer.create_point_buffer(bf_size)
    
    # Assign a default parsing function
    if parse_fn:
        bf.mqtt_parse_function = lambda msg: {'value': msg}

    # Fill and/or return
    if bf_fill_level > 0:
        return fill_point_buffer(bf, bf_fill_level)
    return bf

def create_fill_timeseries(bf_size, bf_fill_level, parse_fn=False):
    # Create
    bf = Buffer.create_timeseries(bf_size)
    
    # Assign a default parsing function
    if parse_fn:
        bf.mqtt_parse_function = lambda msg: {'value': msg}

    # Fill and/or return
    if bf_fill_level > 0:
        return fill_timeseries(bf, bf_fill_level)
    return bf
