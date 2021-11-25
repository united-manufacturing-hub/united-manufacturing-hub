"""
This file provides logging related utilities
"""


class Colors:
    """
    This class provides escape codes for colored terminal output
    """
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'


class Log:
    """
    This class provides colored stdout logging
    """

    @staticmethod
    def log(text: str, color: str):
        print(f"{color}{text}{Colors.ENDC}")

    @staticmethod
    def ok(text: str):
        Log.log(text, Colors.OKGREEN)

    @staticmethod
    def info(text: str):
        Log.log(text, Colors.OKCYAN)

    @staticmethod
    def warn(text: str):
        Log.log(text, Colors.WARNING)

    @staticmethod
    def fail(text: str):
        Log.log(text, Colors.FAIL)
