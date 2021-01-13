# Use an official Python runtime as a parent image
FROM python:3.7-slim

# Set the working directory to /app
WORKDIR /app

# Install any needed packages specified in requirements.txt
COPY ./sensorconnect/requirements.txt ./
RUN pip install --trusted-host pypi.python.org -r requirements.txt --no-cache-dir

# Copy the current directory contents into the container at /app
COPY ./sensorconnect/src /app

# Run app.py when the container launches
CMD ["python","-u", "main.py"]
