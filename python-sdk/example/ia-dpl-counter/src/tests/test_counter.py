import unittest

from DiscreteCounter import DiscreteCounter

import pandas as pd

class TestDplCounter(unittest.TestCase):

    # helper function
    def getDummyBuffer(self, threshold, datapoint_name):
        BROKER_URL = "DUMMY"
        BROKER_PORT = 8883
        CLIENT_ID = "DUMMY_CLIENT_ID"

        CUSTOMER_ID = "DUMMY_CUSTOMER_ID"
        LOCATION = "DUMMY_LOCATION"
        MACHINE_ID = "DUMMY_MACHINE_ID"
        TRANSMITTER_ID = "DUMMY_TRANSMITTER_ID"

        SUB_TOPIC = "DUMMY_SUB_TOPIC"
        PUB_TOPIC = "ia/{}/{}/{}/count".format(CUSTOMER_ID,
                                                LOCATION,
                                                MACHINE_ID)


        return DiscreteCounter.DiscreteCounter(broker_url=BROKER_URL,
                            broker_port=BROKER_PORT,
                            client_id=CLIENT_ID,

                            customer_id=CUSTOMER_ID,
                            location=LOCATION,
                            machine_id=MACHINE_ID,
                            transmitter_id=TRANSMITTER_ID,

                            sub_topic=SUB_TOPIC,
                            pub_topic=PUB_TOPIC,
                            
                            distance_threshold=threshold,
                            datapoint_name=datapoint_name,
                            )

    def test_payload_parsing_1(self):
        bf = self.getDummyBuffer(30, "Distance")

        incomingMQTTMessage = """
        {
            "serial_number": "TEST",
            "Distance": 15,
            "timestamp_ms": 555,
            "mode": 1
        }
        """

        expectedDatapoint = {
            'count': 0,
            'timestamp': pd.to_datetime(555, unit='ms')
        }
        
        actualDatapoint = bf.parseLightBarrier(incomingMQTTMessage)

        self.assertEqual(expectedDatapoint, actualDatapoint)

    def test_payload_parsing_2(self):
        bf = self.getDummyBuffer(30, "Distance")

        incomingMQTTMessage = """
        {
            "serial_number": "TEST",
            "Distance": 32,
            "timestamp_ms": 555,
            "mode": 1
        }
        """

        expectedDatapoint = {
            'count': 0,
            'timestamp': pd.to_datetime(555, unit='ms')
        }
        
        actualDatapoint = bf.parseLightBarrier(incomingMQTTMessage)

        self.assertEqual(expectedDatapoint, actualDatapoint)
    def test_payload_parsing_3(self):
        bf = self.getDummyBuffer(30,"Distance")

        incomingMQTTMessage ="""
        {
            "serial_number": "TEST",
            "Distance": 30,
            "timestamp_ms": 555,
            "mode": 2
        }
        """

        expectedDatapoint = {
            'count': 0,
            'timestamp': pd.to_datetime(555, unit='ms')
        }

        actualDatapoint = bf.parseLightBarrier(incomingMQTTMessage)

        self.assertEqual(expectedDatapoint, actualDatapoint)
    def test_payload_parsing_4(self):
        bf = self.getDummyBuffer(30,"Distance")

        incomingMQTTMessage ="""
        {
            "serial_number": "TEST",
            "Distance": 32,
            "timestamp_ms": 555,
            "mode": 2
        }
        """
        expectedDatapoint = {
            'count': 0,
            'timestamp': pd.to_datetime(555, unit='ms')
        }

        actualDatapoint = bf.parseLightBarrier(incomingMQTTMessage)

        self.assertEqual(expectedDatapoint, actualDatapoint)
    
    def test_buffer_processing_rising_edge(self):
        bf = self.getDummyBuffer(30, "Distance")

        datapoint1 = {
            'count': 0,
            'timestamp': pd.to_datetime(100, unit='ms')
        }

        datapoint2 = {
            'count': 1,
            'timestamp': pd.to_datetime(200, unit='ms')
        }

        buffer = pd.DataFrame([datapoint1, datapoint2])

        expectedPayload = {
            "serial_number": bf.customer_id + "-" + bf.location + "-" + bf.machine_id,
            "timestamp_ms": 200,
            "count": 1
        }
        
        actualPayload = bf.count_up(buffer)

        self.assertEqual(expectedPayload, actualPayload)

    def test_buffer_processing_similar_1(self):
        bf = self.getDummyBuffer(30, "Distance")

        datapoint1 = {
            'count': 0,
            'timestamp': pd.to_datetime(100, unit='ms')
        }

        datapoint2 = {
            'count': 0,
            'timestamp': pd.to_datetime(200, unit='ms')
        }

        buffer = pd.DataFrame([datapoint1, datapoint2])

        expectedPayload = None
        
        actualPayload = bf.count_up(buffer)

        self.assertEqual(expectedPayload, actualPayload)

    def test_buffer_processing_similar_2(self):
        bf = self.getDummyBuffer(30, "Distance")

        datapoint1 = {
            'count': 1,
            'timestamp': pd.to_datetime(100, unit='ms')
        }

        datapoint2 = {
            'count': 1,
            'timestamp': pd.to_datetime(200, unit='ms')
        }

        buffer = pd.DataFrame([datapoint1, datapoint2])

        expectedPayload = None
        
        actualPayload = bf.count_up(buffer)

        self.assertEqual(expectedPayload, actualPayload)

    def test_buffer_processing_falling_edge(self):
        bf = self.getDummyBuffer(30, "Distance")

        datapoint1 = {
            'count': 1,
            'timestamp': pd.to_datetime(100, unit='ms')
        }

        datapoint2 = {
            'count': 0,
            'timestamp': pd.to_datetime(200, unit='ms')
        }

        buffer = pd.DataFrame([datapoint1, datapoint2])

        expectedPayload = None
        
        actualPayload = bf.count_up(buffer)

        self.assertEqual(expectedPayload, actualPayload)

if __name__ == '__main__':
    unittest.main()