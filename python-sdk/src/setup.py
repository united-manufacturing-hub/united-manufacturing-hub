import setuptools

with open("README.md", "r") as fh:
    long_description = fh.read()

setuptools.setup(
    name="dpl",
    version="0.2.0",
    author="Ricardo Vega Ayora",
    author_email="ricardo_vega@dcc-aachen.com",
    description="A data processing library.",
    long_description=long_description,
    long_description_content_type="text/markdown",
    url="//TODO",
    packages=setuptools.find_packages(),
    install_requires=[
        'pandas',
        'paho-mqtt'
    ],
    classifiers=[
        "Programming Language :: Python :: 3",
        "License :: //TODO",
        "Operating System :: OS Independent",
    ],
    python_requires='>=3.7',
)
