"""
Provides useful helper functions
"""
import sys


def memoize(function):
    """
    Runs a function once and caches it's return values for the rest of the runtime
    :param function: function to execute once
    :return: function return value (either by executing it or returning the cached values)
    """
    memo = {}

    def wrapper(*args):
        if args in memo:
            return memo[args]
        else:
            rv = function(*args)
            memo[args] = rv
            return rv

    return wrapper


class Helper:
    """
    Class for all helper functions that have no other place
    """
    @staticmethod
    @memoize
    def supports_unicode():
        """
        Returns if the terminal supports unicode
        :return: terminal supports unicode
        """
        try:
            '┌┬┐╔╦╗╒╤╕╓╥╖│║─═├┼┤╠╬╣╞╪╡╟╫╢└┴┘╚╩╝╘╧╛╙╨╜'.encode(sys.stdout.encoding)
            return True
        except UnicodeEncodeError:
            return False


class Progressbar:
    """
    Provides an dependency free progress bar.
    Do not write anything to stdout, while a progressbar is running !
    """
    def __init__(self, width):
        """
        Initializes the progressbar with set width
        :param width: width of the progressbar
        """
        sys.stdout.write("[%s]" % (" " * width))
        sys.stdout.flush()
        sys.stdout.write("\b" * (width + 1))

    @staticmethod
    def add_progress():
        """
        Adds progress to the last initialized progressbar
        """
        sys.stdout.write("-")
        sys.stdout.flush()

    @staticmethod
    def finish():
        """
        Closes the progressbar
        """
        sys.stdout.write("]\n")
