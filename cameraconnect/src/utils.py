import os
import logging
import sys
from typing import Union


# typing for log levels is compromise between readability

def get_logger(application: str, name: str,
               stdout_level: type(logging.WARNING) = logging.WARNING,
               stderr_level: type(logging.ERROR) = logging.ERROR,
               base_level: type(logging.WARNING) = logging.WARNING,
               log_file: str = None, debug: bool = False,
               file_log_disabled: bool = False,
               log_prefix: str = "PYTHON_") -> logging.Logger:
    """
    Convenience function to get a logger object with unified output style and channels.

    if used with debug=True this will lower the the sys.stdout log level to INFO, and the base log level to DEBUG
    if you wish to get more output for debugging certain apps you can lower their log level setting stdout_level to
    DEBUG or something lower.
    Non standard log levels should not be used especially outside of feature branches.
    Notes:
        File output only works if a file is only associated with 1 unique logger object, do not try to consolidate
        file output of multiple threads / apps into the same file,
        that may crash the application and would be a pain to troubleshoot.
        If you require the logs of multiple sub applications to be in the same file merge multiple outputs files with a
        script.
    Environment:
        Uses the following env variables:
            {log_prefix}LOG_FILE = file location string
            {log_prefix}PYTHON_LOG_DEBUG = true, to force debug to true, everything else does not affect the program
    Args:

        application: name of the application the logger is used in e.g. build_phase
        name: name of the sub part the logger is used in e.g. models
        stdout_level: level of which data is send to sys.stdout

        stderr_level: level of which data is send to sys.stderr
        debug: whether to use debug settings or not, should be false in production code
        base_level: log level of the base logger, also of the file output
        log_file: base log filename to use e.g. log, this will get the name, application and .log appended e.g.
            my_log-cameraconnect.log
            this is overwritten by the environment variable
            this should be None when merged into staging
        file_log_disabled: this disables the file log entirely
        log_prefix: prefix for the environment variables variables, defaults to PYTHON_

    Returns(logging.Logger):
        logger object with all configuration done
    """
    custom_log = False
    if (base_level % 10 != 0) or (stdout_level % 10 != 0) or (stderr_level % 10 != 0):
        custom_log = True
    if os.getenv(f"{log_prefix}LOG_DEBUG", "false").lower() == "true":
        debug = True
    if debug:
        base_level = min(logging.DEBUG, base_level)
        stdout_level = min((logging.INFO, stdout_level, base_level))
    logger = logging.getLogger(f"{application}/{name}")
    logger.setLevel(base_level)

    # create handlers, they can never log anything below the base level

    # handler for sysout
    stdout_handler = logging.StreamHandler(stream=sys.stdout)
    stdout_handler.setLevel(stdout_level)
    # handler for syserr
    stderr_handler = logging.StreamHandler(stream=sys.stderr)
    stderr_handler.setLevel(stderr_level)
    # create formatter and add it to the handlers

    formatter = logging.Formatter('{"level":"%(levelname)s","ts":"%(created)s",'
                                  '"caller":"%(name)s","msg":"%(message)s"}')  # format to be similar to zap
    stdout_handler.setFormatter(formatter)
    stderr_handler.setFormatter(formatter)

    # handler for file output
    env_log_file = os.getenv(f"{log_prefix}LOG_FILE")
    if env_log_file != "":
        log_file = env_log_file

    if (log_file is not None) and (not file_log_disabled):
        file_name = f"{log_file.replace('.log', '')}-{application}-{name}.log"  # creates log files for each namespace
        #  this should work in most cases be aware when logging in multithreaded scenarios
        file_handler = logging.FileHandler(filename=file_name)
        file_handler.setLevel(base_level)  # file always generates the most detailed logs
        logger.addHandler(file_handler)

    # add the handlers to the logger
    logger.addHandler(stdout_handler)
    logger.addHandler(stderr_handler)

    if os.getenv(f"{log_prefix}LOG_DEBUG", "false").lower() == "true":
        logger.info("log level was changed due to env variable")

    if debug:
        logger.info("level was changed due to debug = True")

    if custom_log:
        logger.warning(f"logger is in custom mode with levels base: {base_level} "
                       f"out: {stdout_level} err: {stderr_level}")

    return logger


def get_logger_from_env(application: str, name: str, log_prefix: str = "PYTHON_") -> logging.Logger:
    """
    creates logger object from environment variables
    Args:
        application (): name of the applications this log is a part of e.g. cammeraconnect
        name (): name of the logger inside the application, e.g. class or file this logger is used in
        log_prefix (): prefix of the env variable this logger uses

    Environment:
        Uses the following env variables:
            {log_prefix}LOG_FILE = file location as string
                example:
                    /var/logs/
                        -> /var/logs/<application>-<name>.log
                    /var/logs/test1234.log
                        -> /var/logs/test1234-<application>-<name>.log
            {log_prefix}LOG_DEBUG = true, to force debug to true, everything else does not affect the program
                Valid values:
                    <True> in any capitalisation
                    everything else is counted as false
            {log_prefix}LOG_LEVEL = level for the base and stdout log, this also defines the log level for the file
                Valid values:
                    <DEBUG> in any capitalisation
                    <INFO> in any capitalisation
                    <WARNING> in any capitalisation
                    <ERROR> in any capitalisation
                    <CRITICAL> in any capitalisation

    Examples:
        logger = get_logger_from_env("camera_connect", "main", log_prefix= "PYTHON_"):
        # listens to env variables:
        # PYTHON_LOG_FILE
        # PYTHON_LOG_DEBUG
        # PYTHON_LOG_LEVEL
        # and creates a log entry like
        # {"level":"ERROR","ts":"1635518537.1151698","caller":"camera_connect/main","msg":"asdf"}
    Returns:
        logger(logging.logger): logger object to log with
    """
    logging_level = os.getenv(f'{log_prefix}LOGGING_LEVEL', 'WARNING').upper()
    if logging_level == "DEBUG":
        logger = get_logger(application=application, name=name, base_level=logging.DEBUG,
                            stdout_level=logging.DEBUG, debug=True, log_prefix=log_prefix)
    elif logging_level == "INFO":
        logger = get_logger(application=application, name=name, base_level=logging.INFO,
                            stdout_level=logging.INFO, log_prefix=log_prefix)
    elif logging_level == "WARNING":
        logger = get_logger(application=application, name=name, base_level=logging.WARNING,
                            stdout_level=logging.WARNING, log_prefix=log_prefix)
    elif logging_level == "ERROR":
        logger = get_logger(application=application, name=name, base_level=logging.ERROR,
                            stdout_level=logging.ERROR, log_prefix=log_prefix)
    elif logging_level == "CRITICAL":
        logger = get_logger(application=application, name=name, base_level=logging.CRITICAL,
                            stdout_level=logging.DEBUG, log_prefix=log_prefix)
    else:
        logger = get_logger(application=application, name=name, base_level=logging.WARNING,
                            stdout_level=logging.INFO,
                            log_prefix=log_prefix)  # default value, in case you mess up the setting
        logger.warning(f"log setting invalid, < {logging_level} > is not a valid log setting")

    return logger
