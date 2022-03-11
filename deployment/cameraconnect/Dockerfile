FROM python:3.8-slim-buster
# 3.8 since harvesters is not supported in python 3.9 or 3.10, as of 29.10.21

WORKDIR /app

# Install requirements
COPY ./cameraconnect/requirements.txt /app
RUN pip install -r /app/requirements.txt --no-cache-dir

#necessary for GenICam Cameras manufactured by Allied Vision
RUN apt-get update -y && \
    apt-get install -y --no-install-recommends \
    libgl1-mesa-glx \
    libglib2.0-dev \
    libsm6 \
    libxext6 \
    libxrender1

# Copy the source code
COPY ./cameraconnect/src/* ./

# Run the script
CMD [ "python3", "-u" ,"./main.py" ]
