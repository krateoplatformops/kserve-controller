package storage

type StorageLabel string

const (
	KrateoStorage StorageLabel = "krateo"
)

type StorageInterface any

func GetStorageSpecs() []StorageLabel {
	return []StorageLabel{
		KrateoStorage,
	}
}
