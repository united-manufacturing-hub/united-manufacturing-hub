# Data Processing Library (DPL) [DEPRECATED]
### Python package to temporarily buffer MQTT messages

# Contents
- [Data Processing Library (DPL) [DEPRECATED]](#data-processing-library-dpl-deprecated)
    - [Python package to temporarily buffer MQTT messages](#python-package-to-temporarily-buffer-mqtt-messages)
- [Contents](#contents)
- [Build and installation](#build-and-installation)
  - [Building the Docker image](#building-the-docker-image)
  - [Installing and testing the Python package for local use](#installing-and-testing-the-python-package-for-local-use)
- [Usage in Docker images](#usage-in-docker-images)
- [Documentation](#documentation)
  - [General usage](#general-usage)
  - [Examples](#examples)
    - [Timeseries](#timeseries)
    - [Custom columns](#custom-columns)
    - [User-managed timestamps](#user-managed-timestamps)
  - [Optional MQTT functionality](#optional-mqtt-functionality)
  - [How data is buffered and processed](#how-data-is-buffered-and-processed)
    - [*mqtt_parse_function*](#mqtt_parse_function)
    - [*processing_function*](#processing_function)

# Build and installation
First of all, download the repository:
``` bash
git clone //TODO
```

## Building the Docker image
For x86-64
``` bash
docker build . -t latest
```

For ARM
``` bash
docker build -f ./dpl-arm.dockerfile . -t arm-latest
```

## Installing and testing the Python package for local use
Update setuptools
``` bash
pip install setuptools
```

Install the package
``` bash
cd src
python setup.py install
```

Test after installation
``` bash
cd src/tests
python -m unittest discover
```

# Usage in Docker images
You need to use the DPL docker image as base if you want to enable your containers with the library.

YourNewDockerfile:
``` Dockerfile
FROM <YOUR-TAG>:latest
```
Or
``` Dockerfile
FROM <YOUR-TAG>:arm-latest
```

A manifest list could be used in the future to use only one tag for both architectures.

# Documentation
The buffer class should only be instantiated using the ***MqttBuffer.create_point_buffer*** or the ***MqttBuffer.create_timeseries*** functions. For detailed descriptions about the ***broker_url***, ***broker_port*** and ***cliend_id*** parameters of the constructor, please see the documentation of the Paho MQTT library.

## General usage
1. Import the library.
2. Instantiate a buffer in either the timeseries or the point buffer formats.
3. Pass a parsing and a processing functions to the buffer.
   
The buffer runs in the backgorund, so you might want to have an endless loop in your main script to prevent the program from coming to an end. 

## Examples
``` python
# The following code creates a single column point buffer with an internally-
# managed timestamp.
import json
from dpl.buffer import MqttBuffer as bf

# Parsing function. Parses the incoming MQTT message. Whatever this
# function returns will be stored in the buffer.
def parse_message(message_string):
    # This example assumes that the message_string is a JSON object.
    payload = json.loads(message_string)
    datapoint = {
        'value': payload['my_key']
    }
    return datapoint

# Processing function. Does something with the data the buffer stores
# Whatever this function returns will be published to a specified
# MQTT topic.
def process_buffer(buffer):
    publish_payload = {
      'my_value': buffer['value'].mean()
    }
    return publish_payload

# Create a buffer to store 100 points
my_buffer = bf.create_point_buffer(
    buffer_size=100,
    broker_url='somewhere.over.theinternet',
    sub_topic='my_raw_data_topic',
    pub_topic='my_processed_data_topic',
    client_id='example'
)

my_buffer.mqtt_parse_function = parse_message
my_buffer.processing_function = process_buffer

# Main loop
if __name__ == "__main__":  
    # Loop forever. The logic runs in the background.
    while True:
        time.sleep(1)

```

### Timeseries
Use ***create_timeseries()*** istead of ***create_pointbuffer()***.
``` python
# Create a buffer to all the points that came in within the last 60 seconds
my_buffer = bf.create_timeseries(
    buffer_size=60, # 60 seconds
    broker_url='somewhere.over.theinternet',
    sub_topic='my_raw_data_topic',
    pub_topic='my_processed_data_topic',
    client_id='example'
)
```

### Custom columns
An array of strings should be passed to the ***buffer_columns*** of the ***create_timeseries()*** or the ***create_pointbuffer()*** methods and the parsing and processing functions should be adapted accordingly.
``` python
def parse_message(message_string):
    payload = json.loads(message_string)
    datapoint = {
        'sensor1': payload['my_key1'],
        'sensor2': payload['my_key2']
    }
    return datapoint

def process_buffer(buffer):
    publish_payload = {
      'my_value1': buffer['sensor1'].mean(),
      'my_value2': buffer['sensor2'].mean(),
      'my_value3': buffer['sensor1'].max() + buffer['sensor2'].min() 
    }
    return publish_payload

my_buffer = bf.create_pointbuffer(
    buffer_size=200,
    buffer_columns=['sensor1','sensor2'],
    broker_url='somewhere.over.theinternet',
    sub_topic='my_raw_data_topic',
    pub_topic='my_processed_data_topic',
    client_id='example'
)
```

### User-managed timestamps
The buffer assigns a ***pandas.Timestamp*** to each data point that it stores. It allows, however, users can use the parsing function to store their own timestamps as long as the use the ***pandas.Timestamp*** format.

***IMPORTANT:*** Make sure that your custom timestamps make sense, specially when using timeseries buffers. Unchronological sequences may cause unexpected behaviour.
``` python
def parse_message(message_string):
    payload = json.loads(message_string)
    datapoint = {
        'timestamp': pandas.Timestamp(payload['timestamp'], unit='s')
        'sensor1': payload['my_key']
    }
    return datapoint
```

## Optional MQTT functionality
The buffer needs a broker URL, a subscription topic AND a publication topic to enable the MQTT functionality. If any of the three parameters is missing, the buffer will still be instantiated, but the data has to be manually input and processed by calling the class's ***on_message***, ***mqtt_parse_function*** and/or ***processing_function*** methods.

## How data is buffered and processed
Everytime a new MQTT message is received, the ***on_message*** method is called. See the documentation of the Paho MQTT library for more information. The MqttBuffer's ***on_message*** method first transforms the entire message's payload into a string and then passes it to the ***mqtt_parse_function*** method. Whatever the parsing function returns is then stored in the buffer. After the parsing function returns, the ***processing_function*** method is called to process the entire buffer. Whatever is returned by the processing function is then published to the MQTT topic to the define ***pub_topic***. The ***processing_function*** is called even if the buffer is not full.

### *mqtt_parse_function*
Should take a string as an argument. The string contains the information received from the ***sub_topic***. The user can then define what should be done to the string. Whatever is returned by this function will be stored in the buffer with an automatically generated-timestamp. The user can also pass a self-defined timestamp as long as it complies with the ***pandas.Timestamp*** format. If the user does not want return any value, the parsing function must return ***None*** like in the example below. ***If this function is not defined by the user, no data will be stored in the buffer.***
``` python
def parse_message(message_string):
    payload = json.loads(message_string)
    if payload['my_key'] > threshold:
    	# Save the point if it is above the threshold.
      datapoint = {
          'timestamp': pandas.Timestamp(payload['timestamp'], unit='s')
          'sensor1': payload['my_key']
      }
      return datapoint
    else:
    	# Do not buffer anything if the value is under the threshold
      return None
```

### *processing_function*
Should take a Pandas dataframe as an argument. The incoming dataframe has the structure:
| index | timestamp | [user columns] |
|-------|-----------|-------|
|   0   |           |       |
|   1   |           |       |
|  ...  |           |       |

The ***timestamp*** column holds objects of type ***pandas.Timestamp***. See the Pandas documentation for more information.

The user can then define what to do with the data and what to return. Whatever this function returns is then published to the specified ***pub_topic***. If nothing is to be returned, this function ***MUST*** return ***None***.
