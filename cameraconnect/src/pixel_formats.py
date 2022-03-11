"""
python file for image format handling
"""
from typing import Callable

import cv2
import numpy as np

from harvesters.core import Buffer as HBuffer
from utils import get_logger

logger = get_logger("inference_environment", "pixel_format")


class InvalidPixelFormat(Exception):
    """
    raised when an invalid pixel format is used.
    This is probably due to misconfiguration of the camera or a "special" implementation of the camera vendor.
    Try to use one of the supported formats if that is not possible ask for support on GitHub.
    (see FormatHandler.supported_formats) for available formats
    """


class ImageData:
    """
    dataclass for images

    """

    def __init__(self, image_array: np.ndarray, image_channels: int, original_pixel_format: str):
        """
        class contains image data as np.ndarray and some metadata regarding that image
        Args:
            image_array ():
            image_channels ():
            original_pixel_format ():
        """
        self.image_array = image_array
        self.image_channels = image_channels
        self.original_pixel_format = original_pixel_format

    def __str__(self):
        return f"image with format {self.original_pixel_format}, channels {self.image_channels}"

    @property
    def __bool__(self):
        return (self.image_channels is not None) and \
               (self.image_array is not None) and \
               (self.original_pixel_format is not None)


class FormatHandler:
    """
    handler class to process genicam buffers of different kinds and return BGR or Greyscale images

    """
    supported_formats = ["MONO8",
                         "RGB8",
                         "BGR8",
                         "YUV422_8_UYVY",
                         "YUV422_8",
                         "BAYERRG8",
                         "BAYERBG8",
                         "BAYERGB8"
                         ]

    def __init__(self):
        self.__processing_functions = {
            "MONO8": self.__process_mono8,
            "RGB8": self.__process_rgb8,
            "BGR8": self.__process_bgr8,
            "YUV422_8_UYVY": self.__process_yuv422_8_uyvy,
            "YUV422_8": self.__process_yuv422_8,
            "BAYERRG8": self.__process_bayer_rg_8,
            "BAYERBG8": self.__process_bayer_bg_8,
            "BAYERGB8": self.__process_bayer_gb_8
        }
        self.__image_channels = {
            "MONO8": 1,
            "RGB8": 3,
            "BGR8": 3,
            "YUV422_8_UYVY": 3,
            "YUV422_8": 3,
            "BAYERRG8": 3,
            "BAYERBG8": 3,
            "BAYERGB8": 3
        }

    def get_image_channels(self, pixel_format: str):
        """
        returns the number of color channels for a given image format,
        common values are
        1 for mono images ( greyscale)
        2 as intermediate shape for formats with 422 chroma subsampling -> these should be
            handled by this function and returned as bgr
        3 for color images
        4 for color images with transparency
        Other numbers may be used by specialised equipment


        Args:
            pixel_format (): pixel format of the input buffer

        Returns:

        """
        image_channels = self.__image_channels.get(pixel_format.upper(), None)
        if image_channels is None:
            logger.critical("Pixel format %s not supported by this library", pixel_format)
            logger.info("available formats are : %s", self.supported_formats)
            raise InvalidPixelFormat("Pixel format not supported by this library")
        return image_channels

    def process_buffer(self, buffer: HBuffer) -> ImageData:
        """
        Processes the buffer and returns an image in the as np.ndarray, with dtype uint8
        Args:
            buffer (): Harvesters buffer of the image

        Returns: ImageData:
            in BGR or mono color space as np array the image dimensions depend on the output format and can be gathered
            from get image_channels, they are either [m,n,c] or [m,n] in case c = 1
        """
        pixel_format = buffer.payload.components[0].data_format
        processing_function: Callable[[HBuffer], np.ndarray] = self.__processing_functions.get(pixel_format.upper())

        if processing_function is None:
            logger.critical("Pixel format %s not supported by this library", pixel_format)
            logger.info("Available formats are : %s", self.supported_formats)
            raise InvalidPixelFormat("Pixel format not supported by this library")

        return ImageData(image_array=processing_function(buffer),
                         image_channels=self.get_image_channels(pixel_format),
                         original_pixel_format=pixel_format)

    ############################################
    # Greyscale                                #
    ############################################

    @staticmethod
    def __process_mono8(buffer: HBuffer) -> np.ndarray:
        """
        returns image from  a buffer containing a mono 8
        this results in an [m,n] shaped array

        Args:
            buffer (): harvesters image buffer from the camera

        Returns:
            image_array() retrieved image in MONO as np.ndarray [m,n]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8,
                                     shape=(component.height, component.width))
        return retrieved_image

    ############################################
    # Color with subsampling                   #
    ############################################

    @staticmethod
    def __process_yuv422_8_uyvy(buffer: HBuffer) -> np.ndarray:
        """
        handles yuv422_uyvy extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8, shape=(component.height, component.width, 2))
        # 2 because opencv expects it like this
        retrieved_image = cv2.cvtColor(retrieved_image, cv2.COLOR_YUV2BGR_Y422)
        return retrieved_image

    @staticmethod
    def __process_yuv422_8(buffer: HBuffer) -> np.ndarray:
        """
        handles yuv422 extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(), dtype=np.uint8,
                                     shape=(component.height, component.width, 2))
        retrieved_image = cv2.cvtColor(retrieved_image, cv2.COLOR_YUV2BGR_YUYV)
        return retrieved_image

    ############################################
    # Bayer                                    #
    ############################################
    # mind the discrepancy between opencv Bayer colorspace names and "normal" bayer colorspace names
    # if your images with a bayer colorspace have a weird hue please create an issue on github as this
    # would be manufacturer specific.
    # as a temporary workaround I would recommend you to use a different pixel format if possible
    # (be aware of the additional network costs)
    # if that is not an option you may compensate that in post, by reversing the BAYERWXYZ2BGR conversion with
    # BGR2BAYERWXYZ and then applying another BAYERSTUV2BGR conversion. play around with the other 2 available
    # conversions until you find the correct one best of luck

    @staticmethod
    def __process_bayer_rg_8(buffer: HBuffer) -> np.ndarray:
        """
        handles Bayer_RG8 extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8,
                                     shape=(component.height, component.width, 1))
        retrieved_image = cv2.cvtColor(retrieved_image, cv2.COLOR_BayerRGGB2BGR)
        return retrieved_image

    @staticmethod
    def __process_bayer_bg_8(buffer: HBuffer) -> np.ndarray:
        """
        handles Bayer_BG8 extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8,
                                     shape=(component.height, component.width, 1))
        retrieved_image = cv2.cvtColor(retrieved_image, cv2.COLOR_BayerBGGR2BGR)
        return retrieved_image

    @staticmethod
    def __process_bayer_gb_8(buffer: HBuffer) -> np.ndarray:
        """
        handles Bayer_GB8 extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8,
                                     shape=(component.height, component.width, 1))
        retrieved_image = cv2.cvtColor(retrieved_image, cv2.COLOR_BAYER_GBRG2BGR)
        return retrieved_image

    ############################################
    # Normal RGB Like                          #
    ############################################

    @staticmethod
    def __process_rgb8(buffer: HBuffer) -> np.ndarray:
        """
        handles RGB8 extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8,
                                     shape=(component.height, component.width, 3))
        retrieved_image = cv2.cvtColor(retrieved_image, cv2.COLOR_RGB2BGR)
        return retrieved_image

    @staticmethod
    def __process_bgr8(buffer: HBuffer) -> np.ndarray:
        """
        handles BGR8 extraction and conversion to BGR
        Args:
            buffer (): harvester image buffer

        Returns:
            image_array() retrieved image in BGR as np.ndarray [m,n,c]

        """
        component = buffer.payload.components[0]
        retrieved_image = np.ndarray(buffer=component.data.copy(),
                                     dtype=np.uint8,
                                     shape=(component.height, component.width, 3))
        return retrieved_image
