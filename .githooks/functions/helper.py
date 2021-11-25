import sys


def memoize(function):
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
    @staticmethod
    @memoize
    def supports_unicode():
        try:
            '┌┬┐╔╦╗╒╤╕╓╥╖│║─═├┼┤╠╬╣╞╪╡╟╫╢└┴┘╚╩╝╘╧╛╙╨╜'.encode(sys.stdout.encoding)
            return True
        except UnicodeEncodeError:
            return False


class Progressbar:
    def __init__(self, width):
        sys.stdout.write("[%s]" % (" " * width))
        sys.stdout.flush()
        sys.stdout.write("\b" * (width + 1))

    @staticmethod
    def add_progress():
        sys.stdout.write("-")
        sys.stdout.flush()

    @staticmethod
    def finish():
        sys.stdout.write("]\n")
