# Use an official Python runtime as a parent image
FROM iaproduction.azurecr.io/ia-dpl:latest

# Set the working directory to /app
WORKDIR /app

# Unit testing


# Copy the current directory contents into the container at /app
COPY ./src/ ./

RUN python -m unittest discover

# Run app.py when the container launches
CMD ["python","-u", "main.py"]
