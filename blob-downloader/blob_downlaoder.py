from minio import Minio

client = Minio(
    "play.min.io",
    access_key="Q3AM3UQ867SPQQA43P2F",
    secret_key="zuf+tfteSlswRu7BJ86wekitnifILbZam1KYY3TG",
    secure=True
)
obj = client.get_object(
    "testumh",
    "examples/test.csv",
)
df = pd.read_csv(obj)