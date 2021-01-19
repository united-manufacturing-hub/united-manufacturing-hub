# Use an official Python runtime as a parent image
FROM python:alpine
ENV PYTHONUNBUFFERED 0

# Set the working directory to /app
WORKDIR /app

RUN apk upgrade --update-cache --available && \
    apk add python3-dev gcc musl-dev linux-headers && \
    rm -rf /var/cache/apk/*
    
# Install pyhon requirements
COPY ./requirements.txt /app
RUN pip install --trusted-host pypi.python.org -r requirements.txt --no-cache-dir

# Copy source code
COPY ./src/ .

# Run app.py when the container launches
CMD ["python","-u", "main.py"]
