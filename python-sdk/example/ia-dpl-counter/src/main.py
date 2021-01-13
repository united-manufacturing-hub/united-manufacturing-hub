from DiscreteCounter import DiscreteCounter
import os
import time

# Constants
BROKER_URL = os.environ['BROKER_URL']
BROKER_PORT = int(os.environ['BROKER_PORT'])
CLIENT_ID = os.environ['MQTT_CLIENT_ID']

CUSTOMER_ID = os.environ['CUSTOMER_ID']
LOCATION = os.environ['LOCATION']
MACHINE_ID = os.environ['MACHINE_ID']
TRANSMITTER_ID = os.environ['TRANSMITTER_ID']

SUB_TOPIC = os.environ['SUBSCRIPTION_TOPIC']
PUB_TOPIC = "ia/{}/{}/{}/count".format(CUSTOMER_ID,
                                        LOCATION,
                                        MACHINE_ID)

THRESHOLD = os.environ['THRESHOLD']
DATAPOINT_NAME = os.environ['DATAPOINT_NAME']
MODE = int(os.environ['MODE'])

# Main loop
if __name__ == "__main__":  
    # Subscribe to topic
    bf = DiscreteCounter.DiscreteCounter(broker_url=BROKER_URL,
                        broker_port=BROKER_PORT,
                        client_id=CLIENT_ID,

                        customer_id=CUSTOMER_ID,
                        location=LOCATION,
                        machine_id=MACHINE_ID,
                        transmitter_id=TRANSMITTER_ID,

                        sub_topic=SUB_TOPIC,
                        pub_topic=PUB_TOPIC,
                        
                        distance_threshold=THRESHOLD,
                        datapoint_name=DATAPOINT_NAME,
                        mode=MODE
                        )

    # Loop forever. The logic runs in the background.
    while True:
        time.sleep(1)
    