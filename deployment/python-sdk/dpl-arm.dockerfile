# The build context should be in the dpl directory.

# TODO: Adjust paths

# Use an official Python runtime as a parent image
FROM python:3.9.1-slim
ENV PYTHONUNBUFFERED 0

# Building on a temporary dir
WORKDIR /dpl-temp

# Install building dependencies
RUN apt-get update -y \
    && apt-get install -y libatlas3-base libgfortran5 \
    && apt-get autoremove -y \
    && apt-get clean -y

# Download wheels from piwheels to save timesudo 
RUN touch /etc/pip.conf \
    && echo "[global]" >> /etc/pip.conf \
    && echo "extra-index-url=https://www.piwheels.org/simple" >> /etc/pip.conf

# Install requirements
COPY ./requirements.txt ./
RUN pip install -r requirements.txt --no-cache-dir

# Copy the library files and install the package
COPY ./src/ ./
RUN python setup.py install

# Test the DPL installation
WORKDIR /dpl-temp/tests/
RUN python -m unittest discover

# Change directory to root
WORKDIR /

# Delete source files
RUN rm -rf /dpl-temp

# Create non-root user witht he UID of 1500 and a default home dir /home/ia
RUN useradd -u 1500 -m ia

