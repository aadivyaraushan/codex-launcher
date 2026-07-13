package pairing

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"time"
)

var (
	ErrDeviceNotFound   = errors.New("paired device was not found")
	ErrIdentityNotFound = errors.New("host identity was not found")
)

type DeviceRecord struct {
	ID                string
	Name              string
	PairingGeneration string
	CurrentPublicKey  []byte
	PendingPublicKey  []byte
	PairedAt          time.Time
}

type DeviceInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	PairedAt time.Time `json:"pairedAt"`
}

type PairingOfferRecord struct {
	SecretHash []byte
	Target     PairingTarget
	ExpiresAt  time.Time
	State      string
}

const (
	PairingOfferPending  = "pending"
	PairingOfferClaimed  = "claimed"
	PairingOfferConsumed = "consumed"
)

type Store interface {
	HostIdentity(context.Context) (ed25519.PrivateKey, error)
	SaveHostIdentity(context.Context, ed25519.PrivateKey) error
	Devices(context.Context) ([]DeviceRecord, error)
	Device(context.Context, string) (DeviceRecord, error)
	SaveDevice(context.Context, DeviceRecord) error
	DeleteDevice(context.Context, string) error
	ClearDevices(context.Context) error
	CreatePairingOffer(context.Context, PairingOfferRecord) error
	ClaimPairingOffer(context.Context, []byte, time.Time) (PairingOfferRecord, error)
	ReleasePairingOffer(context.Context, []byte) error
	ConsumePairingOffer(context.Context, []byte) error
}

type MemoryStore struct {
	mu       sync.RWMutex
	identity ed25519.PrivateKey
	devices  map[string]DeviceRecord
	offers   map[string]PairingOfferRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{devices: make(map[string]DeviceRecord), offers: make(map[string]PairingOfferRecord)}
}

func (store *MemoryStore) HostIdentity(ctx context.Context) (ed25519.PrivateKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.identity) == 0 {
		return nil, ErrIdentityNotFound
	}
	return append(ed25519.PrivateKey(nil), store.identity...), nil
}

func (store *MemoryStore) SaveHostIdentity(ctx context.Context, identity ed25519.PrivateKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.identity = append(ed25519.PrivateKey(nil), identity...)
	return nil
}

func (store *MemoryStore) Devices(ctx context.Context) ([]DeviceRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	devices := make([]DeviceRecord, 0, len(store.devices))
	for _, device := range store.devices {
		devices = append(devices, cloneDevice(device))
	}
	return devices, nil
}

func (store *MemoryStore) Device(ctx context.Context, deviceID string) (DeviceRecord, error) {
	if err := ctx.Err(); err != nil {
		return DeviceRecord{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	device, ok := store.devices[deviceID]
	if !ok {
		return DeviceRecord{}, ErrDeviceNotFound
	}
	return cloneDevice(device), nil
}

func (store *MemoryStore) SaveDevice(ctx context.Context, device DeviceRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.devices[device.ID] = cloneDevice(device)
	return nil
}

func (store *MemoryStore) DeleteDevice(ctx context.Context, deviceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.devices[deviceID]; !ok {
		return ErrDeviceNotFound
	}
	delete(store.devices, deviceID)
	return nil
}

func (store *MemoryStore) ClearDevices(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.devices = make(map[string]DeviceRecord)
	return nil
}

func (store *MemoryStore) CreatePairingOffer(ctx context.Context, offer PairingOfferRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	offer.SecretHash = append([]byte(nil), offer.SecretHash...)
	offer.State = PairingOfferPending
	store.offers[string(offer.SecretHash)] = offer
	return nil
}

func (store *MemoryStore) ClaimPairingOffer(ctx context.Context, secretHash []byte, now time.Time) (PairingOfferRecord, error) {
	if err := ctx.Err(); err != nil {
		return PairingOfferRecord{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	offer, ok := store.offers[string(secretHash)]
	if !ok || !now.Before(offer.ExpiresAt) {
		delete(store.offers, string(secretHash))
		return PairingOfferRecord{}, ErrPairingCodeExpired
	}
	if offer.State != PairingOfferPending {
		return PairingOfferRecord{}, ErrPairingCodeUsed
	}
	offer.State = PairingOfferClaimed
	store.offers[string(secretHash)] = offer
	offer.SecretHash = append([]byte(nil), offer.SecretHash...)
	return offer, nil
}

func (store *MemoryStore) ReleasePairingOffer(ctx context.Context, secretHash []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	offer, ok := store.offers[string(secretHash)]
	if ok && offer.State == PairingOfferClaimed {
		offer.State = PairingOfferPending
		store.offers[string(secretHash)] = offer
	}
	return nil
}

func (store *MemoryStore) ConsumePairingOffer(ctx context.Context, secretHash []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	offer, ok := store.offers[string(secretHash)]
	if !ok || offer.State != PairingOfferClaimed {
		return ErrPairingCodeUsed
	}
	offer.State = PairingOfferConsumed
	store.offers[string(secretHash)] = offer
	return nil
}

func cloneDevice(device DeviceRecord) DeviceRecord {
	device.CurrentPublicKey = append([]byte(nil), device.CurrentPublicKey...)
	device.PendingPublicKey = append([]byte(nil), device.PendingPublicKey...)
	return device
}
