"""
Classes to connect, configurate and get image data from cameras.

The module provides two classes:
- GenICam: for all GenICam compatible cameras. Cameras with GigE
            Vision or USB3 Vision transport layer always support
            GenICam.
- DummyCam: for simulating a camera

"""

# Import python in-built libraries
import re
import traceback
from abc import ABC, abstractmethod
import time
import base64
import json

import datetime
import os
import sys

# import other files
import harvesters

from pixel_formats import FormatHandler, ImageData, InvalidPixelFormat
from utils import get_logger_from_env

# Import libraries that had been installed with pip install
import paho.mqtt.client as mqtt
import cv2
import numpy as np

# Import libraries that are only needed for GenICam
from genicam.gentl import TimeoutException
from genicam.genapi import OutOfRangeException
from harvesters.core import Harvester

logger = get_logger_from_env(application="cammeraconnect", name="cameras")

# Console Style elements for outpu
HORIZONTAL_CONSOLE_LINE = "\n" + "_" * 80 + "\n"


class CamGeneral(ABC):
    """
    Abstract base clase for the different cameras.
    This class defines only the basic constructor, the
    method _publish_mqtt() to publish the results to the MQTT
    broker, the method disconnect() and the abstract method
    get_image(). Children must define the get_image() and __del__   method.

    Args of constructor:
        mqtt_host[string]:      Hostname or IP address of the MQTT broker
        mqtt_port[int]:         Network port of the server host to connect to
        mqtt_topic[string]:     Topic on MQTT Broker where trigger signal is send to
                                (e.g. "test/trigger/")

    Returns of constructor:
        See inheritors
    """

    def __init__(self, mqtt_host, mqtt_port, mqtt_topic, mac_address, image_storage_path=None) -> None:
        """
        Base class constructor configures the object with the
        MQTT host settings.

        Args:
            see class description

        Returns:
            see class description
        """

        self.mqtt_host = mqtt_host
        self.mqtt_port = mqtt_port
        self.mqtt_topic = mqtt_topic
        self.mac_address = mac_address
        self.image_storage_path = image_storage_path

        # Connect to the Broker, default port for MQTT 1883
        self.client = mqtt.Client()
        self.client.connect(self.mqtt_host, self.mqtt_port)
        logger.debug("Connected to MQTT broker.")
        self.client.loop_start()

    def _publish_mqtt(self, image: np.ndarray) -> None:
        """
        Sends the timestamp of the time at at which the image was
        taken, and the image itself to the MQTT broker.
        Therefore, the image is first
        converted from a numpy array into a byte array and from
        a byte array into a string.

        The MQTT message contains in one json:
            - timestamp of the acquisition time in ms since epoch
            - image information:
                - string containing the image data (image_bytes)
                - image height, width and channels

        Json format:
            {
            'timestamp_ms': timestamp_ms,
            'image':
                    {'image_id':"<mac_address>_<timestamp>"
                    'image_bytes': encoded_image,
                    'image_height': image.shape[0],
                    'image_width': image.shape[1],
                    'image_channels': image.shape[2] | 1},
            }


        Args:
            image[np.ndarray]:      Array of image in BGR color
                                    format with array size
                                    N x M x image_channels
                                    where N is height, M is width
                                    and image channels the number
                                    of bytes per pixel.
                                    If image_channels is 1 ,
                                    the last dimension is expected to be dropped.
                                    resulting in shape N x M

        Returns:
            None
        """
        # Get timestamp of time  when trigger was received.
        #   Measured in ms since epoch. Epoch is defined as
        #   January 1, 1970, 00:00:00 (UTC)
        timestamp_ms = int(round(time.time() * 1000))

        # Encode numpy array in byte array
        # Use decode() to convert the bytes to a string to send them in a json message
        irrelevant, im_arr = cv2.imencode('.jpg', image)
        im_bytes = im_arr.tobytes()
        encoded_image = base64.b64encode(im_bytes).decode()
        # Preparation of the message that will be published
        # determine image channels, works for both mono and color
        if len(image.shape) == 2:
            channels = 1
        else:
            channels = image.shape[2]
        prepared_message = {
            'timestamp_ms': timestamp_ms,
            'image':
                {'image_id': (str(self.mac_address) + "_" + str(timestamp_ms)),
                 'image_bytes': encoded_image,
                 'image_height': image.shape[0],
                 'image_width': image.shape[1],
                 'image_channels': channels},
        }

        # Get json formatted string, convert python object into
        #   json object

        message = json.dumps(prepared_message)
        # Publish the message
        ret = self.client.publish(self.mqtt_topic, message, qos=0)
        logger.debug("Image No.: " + str(ret[1]))

        logger.debug("Image sent to MQTT broker under topic: " + str(self.mqtt_topic))

    @abstractmethod
    def get_image(self) -> None:
        """
        Must be defined by children.

        Function to get an image from the camera and uses
        _publish_mqtt to send it to MQTT broker.

        Args:
            None

        Returns:
            None
        """
        pass

    def disconnect(self) -> None:
        """
        Disconnects from MQTT broker.

        Args:
            None

        Returns:
            None
        """
        try:
            self.client.loop_stop()
            self.client.disconnect()
        except Exception as _e:
            logger.error("failed to disconnect from mqtt broker with %s \n%s", _e, traceback.format_exc())
        logger.debug("Disconnected from MQTT broker.")

    @abstractmethod
    def __del__(self):
        """
        cleans up the object
        Returns:

        """
        pass


class GenICam(CamGeneral):
    buffer_timeout = 20  # should be achievable for almost all network conditions

    def __init__(self, mqtt_host, mqtt_port, mqtt_topic, mac_address, gen_tl_producer_path_list,
                 user_set_selector="Default", image_width=None, image_height=None, pixel_format=None,
                 image_channels=None, exposure_time=None, exposure_auto=None, gain_auto=None, balance_white_auto=None,
                 image_storage_path=None, acquisition_frame_rate=None) -> None:
        """
            This class is for all GenICam compatible cameras. Cameras
            with GigE Vision or USB3 Vision transport layer always
            support GenICam. Each instance of this class will be
            automatically connected to the device.

            The class inherits all methods from the base class CamGeneral.
            (see description of CamGeneral) Because of that this class has
            to define the abstract method get_image(). In this class
            get_image() fetchs an image out of the buffer/image stream and
            makes sure that data format is BGR8 to send it to MQTT broker
            with _publish_mqtt().

            Additional methods of the GenICam class:
            The first method _connect() establishs a connection to the
            GenICam camera. Afterwards, _apply_settings() applies either
            a configurated user set of configurations or the entered
            settings in the arguments for the class instance. The user
            set of configurations can be created in the matrix vision
            wxPropView or in most SDK which is provided by the camera
            manufacturer. If no user set of configurations is used
            and no settings are provided in the arguments, the default
            settings of the camera will be used.
            The method start _start_acquisition() starts the image stream
            of the camera.
            This class disconnects from the camera after taking an image,
            This reduces the maximum fps but frees the camera for other
            software to connect, and is sometimes more stable

            Args of constructor:
                mqtt_host[string]:          Hostname or IP address of the
                                            MQTT broker
                mqtt_port[int]:             Network port of the server
                                            host to connect to
                mqtt_topic[string]:         Topic on MQTT Broker where
                                            trigger signal is send to
                                            (e.g. "test/trigger/")
                genTL_producer_path[string]:
                                            Path to the *.cti file that
                                            is used to connect to camera
                (opt.) user_set_selector[string]:
                                            Use an already pre-configu-
                                            rated user set.
                                            Possible values: "Default",
                                                "UserSet1", "UserSet2",
                                                "UserSet3", "UserSet4",
                                                "UserSet5" (The number of
                                                user sets is camera
                                                dependent.)
                                            Default value: "Default"
                (opt.) image_width[int]:    Determine in pixels the region
                                            of interest (ROI). ROI will be
                                            always centered in camera
                                            sensor.
                                            A value higher than maximum
                                            resolution of the camera will
                                            set maximum values instead of
                                            the values entered here.
                                            To find out the highest value,
                                            search for the resolution in
                                            the specifications of the
                                            camera.
                                            Specifications are available
                                            in the manual or on the
                                            website where you bought the
                                            camera.
                                            Default: None
                (opt.) image_height[int]:   see image_width
                                            Default: None
                (opt.) pixel_format[string]:
                                            Set the pixel format you want
                                            to use. This program allows
                                            you to take pictures in
                                            monochrome pixel formats
                                            (use: "Mono8") and RGB/BRG
                                            color pixel formats (use:
                                            "RGB8Packed" or "BGR8Packed")
                                            If you only have a camera with
                                            only one image sensor, you can
                                            only take monochrome images.
                                            Possible values: "Mono8",
                                                "RGB8Packed", "BGR8Packed"
                                            Default value: None
                (opt.) image_channels[int]: Number of channels (bytes per
                                            pixel) that are used in the
                                            array (third dimension of the
                                            image data array).You do not
                                            have to set this value.
                                            If None, the best number of
                                            channels for your set pixel
                                            format will be used
                                            Possible Values: 1, 3
                                            Default value: None
                (opt.) exposure_time[float]:
                                            Set the exposure time manually.
                                            Default value: None
                (opt.) exposure_auto[string]:
                                            Determine if camera should
                                            automatically adjust the
                                            exposure time.
                                            Your settings will only be
                                            executed if the camera supports
                                            this. You do not have to check
                                            if the camera supports this.
                                            Possible values are:
                                                - "Off":  No automatic
                                                    adjustment
                                                - "Once": Adjusted once
                                                - "Continuous": Continuous
                                                    adjustment (not
                                                    recommended,
                                                    Attention: This could
                                                    have a big impact on
                                                    the frame rate of your
                                                    camera)
                                            Default value: None
                (opt.) gain_auto[string]:   Determine if camera should
                                            automatically adjust the gain.
                                            Your settings will only be
                                            executed if the camera supports
                                            this. You do not have to check
                                            if the camera supports this.
                                            Possible values are:
                                                see exposure_auto
                                            Default value: None
                (opt.) balance_white_auto[string]:
                                            Determine if camera should
                                            automatically adjust the
                                            white balance.
                                            Your settings will only be
                                            executed if the camera supports
                                            this. You do not have to check
                                            if the camera supports this.
                                            Possible values are:
                                                see exposure_auto
                                            Default value: None
            """
        super().__init__(mqtt_host=mqtt_host,
                         mqtt_port=mqtt_port,
                         mqtt_topic=mqtt_topic,
                         mac_address=mac_address)
        self.acquisition_frame_rate = acquisition_frame_rate
        logger.debug("-" * 80)
        logger.debug(f"initialised {super()} with {mqtt_host} {mqtt_port} {mqtt_topic} {mac_address}")
        self.gen_tl_producer_path_list = gen_tl_producer_path_list
        self.user_set_selector = user_set_selector
        self.image_width = image_width
        self.image_height = image_height
        self.pixel_format = pixel_format
        self.image_channels = image_channels
        self.exposure_time = exposure_time
        self.exposure_auto = exposure_auto
        self.gain_auto = gain_auto
        self.balance_white_auto = balance_white_auto

        self.image_storage_path = image_storage_path

        self.format_handler = FormatHandler()

        # Connect to camera
        self._connect()

        # Apply configurations
        logger.debug("#" * 31 + "applying settings" + "#" * 32)

        self._apply_settings()

        # Start acquisition
        self._start_acquisition()

    def _connect(self) -> None:
        """
        Establishes with the set GenTL Producer a connection to
        the GenICam camera. Also some default settings are done.

        Args:
            None
        Returns:
            None
        """

        # Instantiate a Harvester object to use harvesters core
        self.h = Harvester()

        # Add path of GenTL Producer
        # self.h.add_file(self.gen_tl_producer_path)

        for path in self.gen_tl_producer_path_list:
            self.h.add_file(path)

        # Check if cti-file available, stop if none found
        if len(self.h.files) == 0:
            sys.exit("No valid cti file found")
        logger.debug(HORIZONTAL_CONSOLE_LINE)
        logger.debug("Currently available genTL Producer CTI files: ")
        for file in self.h.files:
            logger.debug(file)

        # Update the list of remote devices; fills up your device
        # information list; multiple devices in list possible
        self.h.update()
        # If no remote devices in the list that you can control
        if len(self.h.device_info_list) == 0:
            sys.exit("No compatible devices detected.")
        # Show remote devices in list
        logger.debug("Available devices:")
        for camera in self.h.device_info_list:
            logger.debug(camera)
        # Create an image acquirer object specifying a target
        # remote device
        # As argument also user_defined_name,
        # vendor, model, etc. possible
        # If multiple cameras in device list, choose the right
        # one by changing the list_index or by using another
        # argument
        first = True  # in case one camera is detected multiple times
        self.__remove_duplicate_entry_from_harvester()
        object_identifier = self.__id_processing(str(self.mac_address))
        for camera in self.h.device_info_list:
            # read cameras mac address
            # ATTENTION: only works with CTI files that deliver the MAC address to harvesters BAUMER SDK
            camera_identifier = self.__id_processing(camera.id_)
            logger.debug(
                f"current device_mac_address: {camera_identifier}, {object_identifier}")

            if not first:
                logger.warning(f"camera {camera} with ident: {camera_identifier} is not first one matching the target "
                               f"id: {self.mac_address}  |"
                               f" ident: {object_identifier}, skipping")
                continue  # using continue instead of break to preserve debug output

            if camera_identifier.find(object_identifier) != -1:
                try:
                    logger.debug(f"attempting to connect to device {camera.id_} ident :{camera_identifier}")
                    self.ia = self.h.create_image_acquirer(id_=camera.id_)
                    first = False
                except Exception as _e:
                    logger.error(
                        "Camera is not reachable. Most likely another container already occupies the same camera. "
                        f"One camera can only be used by exactly one container at any time. {_e}")
                    sys.exit("Camera not reachable.")
                logger.debug(f"Using: {camera} with ident {camera_identifier}")
                logger.debug(HORIZONTAL_CONSOLE_LINE)

        if not hasattr(self, "ia"):
            logger.error(
                "No camera with the specified MAC address available. Please specify MAC address in env file correctly.")
            logger.info(f"attempted to connect to cameras: "
                        f"{[(camera.id_, self.__id_processing(str(camera.id_))) for camera in self.h.device_info_list]}"
                        f"this object has mac_address {self.mac_address} and ident: {object_identifier}")
            sys.exit("Unknown or Invalid MAC address.")
        ## Set some default settings
        # This is required because of a bug in the harvesters
        #   module. This should not affect our usage of image
        #   acquirer. Only change if you know what you are doing
        self.ia.remote_device.node_map.ChunkModeActive.value = False

        # The number of buffers that is prepared for the image
        #   acquisition process. The buffers will be announced
        #   to the target GenTL Producer. Need this so that we
        #   always get the correct actual image.
        self.ia.num_buffers = 3  # test for stemmer imaging todo

    @classmethod
    def __id_processing(cls, identifier: str) -> str:
        """
        helper func to unify pre processing of identifier / mac addresses to ensure compatibility with different CTI
        Files, this is not exhaustive, so if you can not use your hardware with these expressions please create an
        issue on github
        :params:
        identifier: str : input string
        :return:
        string capitalized with different things removed.
        """
        upper_id = identifier.upper()
        device = re.compile("(DEVICEMODULE?)|(DEV)")  # removes common pre/suffixes
        no_dev_id = device.sub("", upper_id)
        spacer_symbols = re.compile("[-.:,;_\s]")  # removes variable spacers used on different cameras
        no_spacer_symbols = spacer_symbols.sub("", no_dev_id)
        return no_spacer_symbols

    def __remove_duplicate_entry_from_harvester(self):
        """
        removes duplicate entries from the harvester camera list,
        required for stemmer imaging under widows with alied vision cameras, probably also for others
        """
        new_list = []
        for d in self.h.device_info_list:
            if any([n_d.id_ == d.id_ for n_d in new_list]):
                continue
            else:
                new_list.append(d)
        self.h._device_info_list = new_list

    def publish_node_map(self, topic=None):
        """

        publishes node map of connected camera to mqtt for debugging
        Returns:
        """
        try:
            if topic is None:
                topic_root = "/".join(self.mqtt_topic.split("/")[:-1])
                topic = f"{topic_root}/node_map"
            node_map = json.dumps(self.get_node_map_dict())
            self.client.publish(topic=topic, payload=node_map, qos=2)
        except Exception as _e:
            logger.error("failed to publish node map with %s\n%s", _e, traceback.format_exc())

    def get_node_map_dict(self) -> dict:
        """
        returns a dict of the genicam nodemap, nodes with unavailable values will have the placeholder
        "py_value_unavailable"
        Returns:

        """
        if self.ia is None:
            logger.error("acquisition not started no node map available")
            return {}
        available_nodes = dir(self.ia.remote_device.node_map)
        node_settings = {}
        for a_n in available_nodes:
            # noinspection PyBroadException
            try:
                node_settings[a_n] = self.ia.remote_device.node_map.get_node(a_n).value
            except Exception:  # should never crash during node generation
                node_settings[a_n] = "py_value_unavailable"
        return node_settings

    def _apply_settings(self) -> None:
        """
        Applies the settings for the camera.

        Either a configured user set of configurations or the
        entered settings in the arguments for the class instance.
        The user set of configurations can be created in the
        matrix vision wxPropView or in most SDK which is provided
        by the camera manufacturer.
        If no user set of configurations is used and no settings
        are provided in the arguments, the default settings of
        the cameras will be used.

        The automatic adjust settings are only applied if camera
        supports these features.

        Args:
            None

        Returns:
            None
        """
        # restarts acquisition if image acquisition was running before
        was_acquiring = False
        if self.ia.is_acquiring():
            self._stop_acquisition()
            was_acquiring = True

        # Get list of all available features of the camera
        node_map = dir(self.ia.remote_device.node_map)
        logger.debug("Adjustable parameters for connected camera:")
        for setting in node_map:
            logger.debug(setting)

        # If camera was already configured and configurations
        # has been saved in user set, then set and load user
        # set here and return
        if self.user_set_selector != "Default":
            self.ia.remote_device.node_map.UserSetSelector.value = self.user_set_selector
            self.ia.remote_device.node_map.UserSetLoad.execute()
            # Do not execute the code afterwards in this function
            #   if user-set is used
            return

        # Set Width
        if self.image_width is not None:
            if self.image_width > self.ia.remote_device.node_map.WidthMax.value:
                # Value given in settings higher than max
                #   -> set max
                self.ia.remote_device.node_map.Width.value = self.ia.remote_device.node_map.WidthMax.value
            else:
                # Set value given in settings
                self.ia.remote_device.node_map.Width.value = self.image_width

        # Set Height
        if self.image_height is not None:
            if self.image_height > self.ia.remote_device.node_map.HeightMax.value:
                # Value given in settings higher than max
                #   -> set max
                self.ia.remote_device.node_map.Height.value = self.ia.remote_device.node_map.HeightMax.value
            else:
                # Set value given in settings
                self.ia.remote_device.node_map.Height.value = self.image_height

        # Set ROI always centered in camera sensor
        # Therefore calculate Offset X and Offset Y where the
        #   readout region should start and assign it to features
        if self.user_set_selector != "Default":
            self.ia.remote_device.node_map.OffsetX.value = int(
                (self.ia.remote_device.node_map.WidthMax.value - self.ia.remote_device.node_map.Width.value) / 2)
            self.ia.remote_device.node_map.OffsetY.value = int(
                (self.ia.remote_device.node_map.HeightMax.value - self.ia.remote_device.node_map.Height.value) / 2)

        # Set PixelFormat
        if self.pixel_format is not None:
            self.ia.remote_device.node_map.PixelFormat.value = self.pixel_format

        # Set Exposure time
        logger.debug(f"exposure auto :{self.exposure_auto} , exposure time {self.exposure_time}")
        if self.exposure_auto is not None:
            try:
                self.ia.remote_device.node_map.ExposureTimeAbs.value = self.exposure_time
            except OutOfRangeException:
                logger.error("Specified Exposure time too high for selected camera. Please choose smaller value.")
                sys.exit(1)

        # Set ExposureAuto, GainAuto and BalanceWhiteAuto;
        #   it always first checks if connected camera supports
        #   this function
        if self.exposure_auto is not None:
            if "ExposureAuto" in node_map:
                self.ia.remote_device.node_map.ExposureAuto.value = self.exposure_auto
            else:
                logger.warning("Camera does not support automatic adjustment of exposure time")
        if self.gain_auto is not None:
            if "GainAuto" in node_map:
                self.ia.remote_device.node_map.GainAuto.value = self.gain_auto
            else:
                logger.warning("Camera does not support automatic adjustment of gain")
        if self.balance_white_auto is not None:
            if "BalanceWhiteAuto" in node_map:
                self.ia.remote_device.node_map.BalanceWhiteAuto.value = self.balance_white_auto
            else:
                logger.warning("Camera does not support automatic adjustment of white balance")

        if self.acquisition_frame_rate is not None:
            if "BalanceWhiteAuto" in node_map:
                self.ia.remote_device.node_map.AcquisitionFrameRateAbs.value = self.acquisition_frame_rate
            else:
                logger.info("Camera does not support acquisition frame rate adjustment")

        # part 2 for continued acquisition
        if was_acquiring:
            self._start_acquisition()
            logger.debug("continuing acquisition after settings")

    def _start_acquisition(self) -> None:
        """
        Activate an image stream from camera to be able to fetch
        images out of stream.

        Args:
            None

        Returns:
            None
        """
        # locks transport parameters in accordance with genciam spec
        # this means you can not set cmera settings after starting an acquisition.
        self.ia.remote_device.node_map.TLParamsLocked.value = 1

        # Starts image acquisition with harvesters
        self.ia.start_acquisition()
        logger.debug("Acquisition started.")

    def _stop_acquisition(self) -> None:
        """
        stops image acquisition

        Args:
            None

        Returns:
            None
        """
        # stops image acquisition with harvesters
        self.ia.stop_acquisition()
        logger.info("Acquisition stopped.")

    # Get image out of image stream
    def get_image(self) -> None:
        """
        Fetch an image out of the image stream and make sure if
        colored image that BGR pixel format is used.

        Args:
            None

        Returns:
            None
        """
        # Try to fetch a buffer that has been filled up with an
        #   image
        logger.debug(" %sget image%s", "#" * 36, "#" * 35)

        try:
            # Default value
            retrieved_image = None
            # start acquisition if necessary
            if self.ia is None:
                self._connect()
                self._apply_settings()
            if not self.ia.is_acquiring():
                self._start_acquisition()

            # To solve the problem that buffer is already filled
            #   with an old image, but we want the newest image,
            #   This here is probably not the best way to solve
            #   the problem. It is a workaround.
            with self.ia.fetch_buffer(timeout=self.buffer_timeout) as buffer:
                # Do not use this buffer, use the next one
                pass
                logger.debug(f"buffer {buffer}")

            # Due to with statement buffer will automatically be
            #   queued
            with self.ia.fetch_buffer(timeout=20) as buffer:
                logger.debug(HORIZONTAL_CONSOLE_LINE)
                logger.debug("Image fetched.")
                # Create an alias of the 2D image component:
                # processes the image data and makes them into the BGR color format
                image_data: ImageData = self.format_handler.process_buffer(buffer=buffer)

                retrieved_image: np.ndarray = image_data.image_array

            self._publish_mqtt(retrieved_image)
            logger.debug("Image converted and published to MQTT.")

            # Save image
            if self.image_storage_path:
                timestamp = datetime.datetime.now(tz=datetime.timezone.utc).isoformat().replace(":", "_").replace(".",
                                                                                                                  "_").replace(
                    "+", "_")
                img_save_dir = os.path.join(self.image_storage_path, "{}.jpg".format(timestamp))
                cv2.imwrite(img_save_dir, retrieved_image)

                logger.debug("Image saved to {}".format(img_save_dir))

            # free camera for other resources
            self.disconnect_camera()

        # If TimeoutException because no image was fetchable,
        # restart the acquisition process
        except InvalidPixelFormat:
            logger.error("pixel format %s not supported by library please use a different one")
            logger.error()
            logger.error("supported formats are: %s ", self.format_handler.supported_formats)

        except TimeoutException:
            logger.error("Timeout ocurred during fetching an image. Camera reset and restart.")

            self.publish_node_map()

            self.disconnect_camera()
            logger.debug("Camera restarted. Ready to fetch an image.")
        # wildcard has to be handled in trigger if so desired

    def disconnect_camera(self):
        """
        disconnects the camera and resets the harvesters module
        Returns:

        """
        try:
            if self.ia is not None:
                try:
                    self._stop_acquisition()
                    self.ia.destroy()
                    logger.debug("image acquirer destroyed")
                except Exception as _e:
                    logger.error("failed to stop acquisition with %s, %s", _e, traceback.format_exc())
            self.h.reset()
        except Exception as _e:
            logger.error("failed to disconnect from camera with %s \n%s", _e, traceback.format_exc())
        logger.debug("harvester reset")

    def disconnect(self) -> None:
        """
        Deactivate acquisition and disconnect from camera.

        Args:
            None

        Returns:
            None
        """
        # Close the connection to ...
        # ... MQTT (and stop loop)

        super().disconnect()

        # disconnect camera

        self.disconnect_camera()

        logger.debug("Disconnected from GenICam camera.")

    def __del__(self):
        """
        destroys genicam and mqtt connection on object deletion
        Returns:

        """
        try:
            self.disconnect()
        except Exception as _e:
            logger.critical("Failed to destroy connection with %s \n%s", _e, traceback.format_exc())


class FasterGenICam(GenICam):
    """
    version of genicam that stays connected wit the camera indefinitely
    """
    buffer_timeout = 5  # more aggressive timeout, may be an issue in slow network environments
    # (where this is thw wrong class anyway)

    max_skipped_frames = 5  # maximum number of consequtive skipped frames before the acquisition is restarted

    # this should not be used then, fall back to standard genicam
    def __init__(self, mqtt_host, mqtt_port, mqtt_topic, mac_address, gentl_producer_path_list,
                 image_width=None, image_height=None, pixel_format=None,
                 image_channels=None, exposure_time=None, exposure_auto=None, gain_auto=None, balance_white_auto=None,
                 image_storage_path=None, acquisition_frame_rate=None):
        super().__init__(mqtt_host=mqtt_host, mqtt_port=mqtt_port, mqtt_topic=mqtt_topic, mac_address=mac_address,
                         gen_tl_producer_path_list=gentl_producer_path_list,
                         image_width=image_width, image_height=image_height, pixel_format=pixel_format,
                         image_channels=image_channels, exposure_time=exposure_time, exposure_auto=exposure_auto,
                         gain_auto=gain_auto, balance_white_auto=balance_white_auto,
                         image_storage_path=image_storage_path,
                         acquisition_frame_rate=acquisition_frame_rate)
        self.continuously_skipped_frames = 0
        self.skipped_frames = 0
        self._connect()
        self._apply_settings()
        self._start_acquisition()

    def __skip_frame(self):
        """
        skips a frame and related crash handling
        Returns:
        """
        self.skipped_frames += 1
        self.continuously_skipped_frames += 1
        logger.error("ressource not available, skipping frame, skipped frames = %s / %s total: %s",
                     self.continuously_skipped_frames,
                     self.max_skipped_frames, self.skipped_frames)
        if self.continuously_skipped_frames > self.max_skipped_frames:
            self.disconnect()  # handles mqtt disconnection as well to prevent the loop from calling back on
            # dead harvesters components
            self.continuously_skipped_frames = 0
            logger.error("resetting due to skipped frames")

    def get_image(self, export_node_map=False) -> None:
        """
        Fetch an image out of the image stream and make sure if
        colored image that BGR pixel format is used.

        Args:
            export_node_map (): exports node map to file

        Returns:
            None
        """

        # Try to fetch a buffer that has been filled up with an
        # image

        try:
            # ensure connection is existing
            if self.ia is None:
                self._connect()
                self._apply_settings()
            if not self.ia.is_acquiring():
                self._start_acquisition()
            # skipping sac buffer for more speed
            with self.ia.fetch_buffer(timeout=self.buffer_timeout) as buffer:
                if len(buffer.payload.components) == 0:
                    logger.error("buffer has no component, skipping frame")
                    self.__skip_frame()
                    return
                # get image data from buffer
                image_data = self.format_handler.process_buffer(buffer=buffer)
                retrieved_image: np.ndarray = image_data.image_array
                self.image_channels: int = image_data.image_channels
            self.continuously_skipped_frames = 0
            logger.debug(f"buffer closed")
            self._publish_mqtt(retrieved_image)

        except harvesters.core.NotAvailableException:  # exception occures immediately, race condition with other
            # images basically impossible.
            self.__skip_frame()
            self.publish_node_map()
        except TimeoutException:
            logger.error("Timeout occurred during fetching an image. Camera reset and restart.")
            self.publish_node_map()
            self.disconnect()
            logger.info("Camera restarted. Ready to fetch an image.")
            self.__skip_frame()
        # wildcard has to be handled in trigger


class DummyCamera(CamGeneral):

    def get_image(self) -> None:
        # Default value
        retrieved_image = None

        # reads a static image which is part of stack
        img = cv2.imread("/app/assets/dummy_image.jpg")
        logger.debug(HORIZONTAL_CONSOLE_LINE)
        logger.debug("Image fetched.")
        height, width, channels = img.shape
        retrieved_image = np.ndarray(buffer=img,
                                     dtype=np.uint8,
                                     shape=(height, width, channels))

        self._publish_mqtt(retrieved_image)
        logger.debug("Image converted and published to MQTT.")

        # Save image
        if self.image_storage_path:
            timestamp = datetime.datetime.now(tz=datetime.timezone.utc).isoformat().replace(":", "_").replace(".",
                                                                                                              "_").replace(
                "+", "_")
            img_save_dir = os.path.join(self.image_storage_path, "{}.jpg".format(timestamp))
            cv2.imwrite(img_save_dir, retrieved_image)

            logger.debug("Image saved to {}".format(img_save_dir))

    def __del__(self):
        pass  # nothing special to do for this class
