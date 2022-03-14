package processors

type AddOrder struct{}

func (AddOrder) ProcessMessage(customerID string, location string, assetID string, payload []byte) (err error) {

	return nil
}
