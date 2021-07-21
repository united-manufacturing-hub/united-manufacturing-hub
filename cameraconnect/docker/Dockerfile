FROM python:3.6-slim-buster

WORKDIR /app

# Install requirements
COPY ./requirements.txt ./
RUN pip install -r ./requirements.txt --no-cache-dir

#necessary for GenICam Cameras manufactured by Allied Vision
RUN apt-get update -y && \
    apt-get install -y --no-install-recommends \
    libgl1-mesa-glx \
    libglib2.0-dev \
    libsm6 \
    libxext6 \
    libxrender1

# Copy the source code
COPY ./src/* ./

# Run the script
CMD [ "python3", "-u" ,"./main.py" ]
