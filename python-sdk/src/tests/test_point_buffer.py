import unittest

from dpl.buffer import MqttBuffer as Buffer

import dummy_test_classes as dtc

# Test cases
class TestBufferMQTTMethods(unittest.TestCase):       
    def test_point_buffer_empty(self):
        # The buffer should remain empty if no messages come
        bf = dtc.create_fill_point_buffer(bf_size=10, bf_fill_level=0)
        self.assertEqual(bf.buffer.size, 0)

    def test_point_buffer_wo_functions(self):
        # Should do nothing because the parsing and processing
        # functions have not been defined
        bf = dtc.create_fill_point_buffer(bf_size=10, bf_fill_level=10)
        self.assertEqual(bf.buffer.size, 0)

    def test_point_buffer_partially_full(self):
        # The buffer should not be full
        bf = dtc.create_fill_point_buffer(bf_size=10, bf_fill_level=4, parse_fn=True)
        
        # Assert. Dividing by 2 to take the number of rows
        # 2 is the default for timestamp and value
        self.assertEqual(bf.buffer.size/len(bf.buffer.columns), 4)
        
    def test_point_buffer_full(self):
        # The buffer should be the same number of elements as the size
        bf = dtc.create_fill_point_buffer(bf_size=10, bf_fill_level=10, parse_fn=True)

        # Assert. Dividing by 2 to take the number of rows
        # 2 is the default for timestamp and value
        self.assertEqual(bf.buffer.size/len(bf.buffer.columns), 10)

    def test_point_buffer_full_function_true(self):
        # Function should return true if the buffer is full
        bf = dtc.create_fill_point_buffer(bf_size=10, bf_fill_level=10, parse_fn=True)

        # Assert. 
        self.assertTrue(bf._buffer_is_full())

    def test_point_buffer_full_function_false(self):
        # Function should return false if the buffer is not full
        bf = dtc.create_fill_point_buffer(bf_size=10, bf_fill_level=4, parse_fn=True)

        # Assert. 
        self.assertFalse(bf._buffer_is_full())

    def test_point_buffer_custom_columns(self):
        # Create buffer
        buffer_size = 10
        bf = Buffer.create_point_buffer(buffer_size, buffer_columns=['value1', 'value2'])

        # Assert for 3 columns including the default timestamp
        self.assertEqual(len(bf.buffer.columns), 3)

    def test_point_buffer_custom_columns_partially_full(self):
        # The buffer should not be full
        bf = Buffer.create_point_buffer(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_point_buffer(bf, 4)
        
        # Assert. Dividing by the number of columns to get the number of rows
        self.assertEqual(bf.buffer.size/len(bf.buffer.columns), 4)
        
    def test_point_buffer_custom_columns_full(self):
        # The buffer should be the same number of elements as the size
        bf = Buffer.create_point_buffer(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_point_buffer(bf, 10)

        # Assert. Dividing by the number of columns to get the number of rows
        self.assertEqual(bf.buffer.size/len(bf.buffer.columns), 10)

    def test_point_buffer_custom_columns_full_function_true(self):
        # Function should return true if the buffer is full
        bf = Buffer.create_point_buffer(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_point_buffer(bf, 10)

        # Assert. 
        self.assertTrue(bf._buffer_is_full())

    def test_point_buffer_custom_columns_full_function_false(self):
        # Function should return false if the buffer is not full
        bf = Buffer.create_point_buffer(10, buffer_columns=['value1', 'value2'])
        bf.mqtt_parse_function = lambda msg: {'value1': 0, 'value2': 1} # 0 and 1 are dummy values
        bf = dtc.fill_point_buffer(bf, 4)

        # Assert. 
        self.assertFalse(bf._buffer_is_full())


if __name__ == '__main__':
    unittest.main()