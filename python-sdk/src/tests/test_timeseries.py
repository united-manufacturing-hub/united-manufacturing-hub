import time
import unittest

import pandas as pd

from dpl.buffer import MqttBuffer as Buffer

import dummy_test_classes as dtc

# Test cases
class TestBufferMQTTMethods(unittest.TestCase):        
    def test_timeseries_empty(self):
        # The buffer should remain empty if no messages come
        bf = dtc.create_fill_timeseries(bf_size=10, bf_fill_level=0)
        self.assertEqual(bf.buffer.size, 0)

    def test_timeseries_wo_functions(self):
        # Should do nothing because the parsing and processing
        # functions have not been defined
        bf = dtc.create_fill_timeseries(bf_size=5, bf_fill_level=5)
        self.assertEqual(bf.buffer.size, 0)

    def test_timeseries_partially_full(self):
        # The buffer should not be full
        bf = dtc.create_fill_timeseries(bf_size=5, bf_fill_level=4, parse_fn=True)
        
        # Assert. Using timestamp for timeseries
        prev_timestamp = bf.buffer.iloc[0]['timestamp']
        curr_timestamp = bf.buffer.iloc[-1]['timestamp']
        # Current timestamp should be less than first + limit
        self.assertLess(curr_timestamp, prev_timestamp + pd.Timedelta(seconds=5))
        
    def test_timeseries_full(self):
        # The buffer should be the same number of elements as the size
        bf = dtc.create_fill_timeseries(bf_size=5, bf_fill_level=5, parse_fn=True)

        # Assert. Using timestamp for timeseries
        prev_timestamp = bf.buffer.iloc[0]['timestamp']
        curr_timestamp = bf.buffer.iloc[-1]['timestamp']
        # Current timestamp should be >= first timestamp + timelimit to be full
        self.assertGreaterEqual(curr_timestamp, prev_timestamp + pd.Timedelta(seconds=5))

    def test_timeseries_full_function_true(self):
        # Function should return true if the buffer is full
        bf = dtc.create_fill_timeseries(bf_size=5, bf_fill_level=5, parse_fn=True)

        # Assert. 
        self.assertTrue(bf._buffer_is_full())

    def test_timeseries_full_function_false(self):
        # Function should return false if the buffer is not full
        bf = dtc.create_fill_timeseries(bf_size=5, bf_fill_level=4, parse_fn=True)

        # Assert. 
        self.assertFalse(bf._buffer_is_full())

    def test_timeseries_custom_columns(self):
        # Create buffer
        buffer_size = 5
        bf = Buffer.create_timeseries(buffer_size, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_timeseries(bf, buffer_size)

        # Assert for 3 columns including the default timestamp
        self.assertEqual(len(bf.buffer.columns), 3)

    def test_timeseries_custom_columns_partially_full(self):
        # The buffer should not be full
        bf = Buffer.create_timeseries(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_timeseries(bf, 4)
        
        # Assert. Using timestamp for timeseries
        prev_timestamp = bf.buffer.iloc[0]['timestamp']
        curr_timestamp = bf.buffer.iloc[-1]['timestamp']
        # Current timestamp should be less than first + limit
        self.assertLess(curr_timestamp, prev_timestamp + pd.Timedelta(seconds=5))
        
    def test_timeseries_custom_columns_full(self):
        # The buffer should be the same number of elements as the size
        bf = Buffer.create_timeseries(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_timeseries(bf, 10)

        # Assert. Using timestamp for timeseries
        prev_timestamp = bf.buffer.iloc[0]['timestamp']
        curr_timestamp = bf.buffer.iloc[-1]['timestamp']
        # Current timestamp should be >= first timestamp + timelimit to be full
        self.assertGreaterEqual(curr_timestamp, prev_timestamp + pd.Timedelta(seconds=5))

    def test_point_timeseries_columns_full_function_true(self):
        # Function should return true if the buffer is full
        bf = Buffer.create_timeseries(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_timeseries(bf, 10)

        # Assert. 
        self.assertTrue(bf._buffer_is_full())

    def test_timeseries_custom_columns_full_function_false(self):
        # Function should return false if the buffer is not full
        bf = Buffer.create_timeseries(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_timeseries(bf, 4)

        # Assert. 
        self.assertFalse(bf._buffer_is_full())

if __name__ == '__main__':
    unittest.main()