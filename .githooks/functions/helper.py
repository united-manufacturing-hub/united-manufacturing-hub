"""
Provides useful helper functions
"""
import subprocess

import sys
import shutil
from functions.log import Log


def check_installed_exe(name: str):
    if shutil.which(name) is None:
        Log.fail(f"{name} is not installed or not in path")
        exit(1)


def install_if_missing(name: str, installation_command: [str]):
    if shutil.which(name) is None:
        Log.warn(f"{name} is not installed, attempting to install")
        p = subprocess.Popen(installation_command, stdin=subprocess.PIPE,
                             stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        output, err = p.communicate()
        rc = p.returncode

        if rc != 0:
            Log.fail(f"Failed to install {name}")
            Log.fail(f"stdout: {output}")
            Log.fail(f"stderr: {err}")
            exit(1)
        Log.info(f"{name} installed successfully")

def check_requirements():
    Log.info("Checking test requirements")
    try:
        import yamllint
    except ImportError:
        Log.fail("Failed to import yamllint")
        Log.fail("Open https://github.com/adrienverge/yamllint to find out how to install with your package manager")
        Log.fail("Or use 'pip install --user yamllint'")
        exit(1)

    check_installed_exe("docker")
    check_installed_exe("go")
    install_if_missing("staticcheck", ["go", "install", "honnef.co/go/tools/cmd/staticcheck@latest"])


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
