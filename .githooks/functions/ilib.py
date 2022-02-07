"""
This file provides the LibInterface interface
"""


class LibInterface:
    """
    This interface must be implemented by any linter
    """

    # Run executes check and returns report
    @classmethod
    def run(cls): raise NotImplementedError

    # Execute checks
    @classmethod
    def check(cls): raise NotImplementedError

    # Reports errors and warnings and returns number of errors
    @classmethod
    def report(cls): raise NotImplementedError
