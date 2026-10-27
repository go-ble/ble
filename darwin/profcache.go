package darwin

// profcache: Profile Cache.  This allows a device to match profile objects
// with their corresponding CoreBluetooth objects (e.g., ble.Servive <->
// cbgo.Service).

import (
	"fmt"
	"sync"

	"github.com/go-ble/ble"
	"github.com/JuulLabs-OSS/cbgo"
)

type profCache struct {
	mtx sync.RWMutex

	svcCbMap map[*ble.Service]cbgo.Service
	chrCbMap map[*ble.Characteristic]cbgo.Characteristic
	dscCbMap map[*ble.Descriptor]cbgo.Descriptor

	cbSvcMap map[cbgo.Service]*ble.Service
	cbChrMap map[cbgo.Characteristic]*ble.Characteristic
	cbDscMap map[cbgo.Descriptor]*ble.Descriptor
}

func newProfCache() profCache {
	return profCache{
		svcCbMap: map[*ble.Service]cbgo.Service{},
		chrCbMap: map[*ble.Characteristic]cbgo.Characteristic{},
		dscCbMap: map[*ble.Descriptor]cbgo.Descriptor{},

		cbSvcMap: map[cbgo.Service]*ble.Service{},
		cbChrMap: map[cbgo.Characteristic]*ble.Characteristic{},
		cbDscMap: map[cbgo.Descriptor]*ble.Descriptor{},
	}
}

func (pc *profCache) addSvc(s *ble.Service, cbs cbgo.Service) {
	pc.mtx.Lock()
	defer pc.mtx.Unlock()

	pc.svcCbMap[s] = cbs
	pc.cbSvcMap[cbs] = s
}

func (pc *profCache) addChr(c *ble.Characteristic, cbc cbgo.Characteristic) {
	pc.mtx.Lock()
	defer pc.mtx.Unlock()

	pc.chrCbMap[c] = cbc
	pc.cbChrMap[cbc] = c
}

func (pc *profCache) addDsc(d *ble.Descriptor, cbd cbgo.Descriptor) {
	pc.mtx.Lock()
	defer pc.mtx.Unlock()

	pc.dscCbMap[d] = cbd
	pc.cbDscMap[cbd] = d
}

func (pc *profCache) findCbSvc(s *ble.Service) (cbgo.Service, error) {
	pc.mtx.RLock()
	defer pc.mtx.RUnlock()

	cbs, ok := pc.svcCbMap[s]
	if !ok {
		return cbs, fmt.Errorf("no CB service with UUID=%v", s.UUID)
	}

	return cbs, nil
}

func (pc *profCache) findSvc(cbs cbgo.Service) (*ble.Service, error) {
	pc.mtx.RLock()
	defer pc.mtx.RUnlock()

	s, ok := pc.cbSvcMap[cbs]
	if !ok {
		return nil, fmt.Errorf("no service with UUID=%v", cbs.UUID())
	}

	return s, nil
}

func (pc *profCache) findCbChr(c *ble.Characteristic) (cbgo.Characteristic, error) {
	pc.mtx.RLock()
	defer pc.mtx.RUnlock()

	cbc, ok := pc.chrCbMap[c]
	if !ok {
		return cbc, fmt.Errorf("no CB characteristic with UUID=%v", c.UUID)
	}

	return cbc, nil
}

func (pc *profCache) findChr(cbc cbgo.Characteristic) (*ble.Characteristic, error) {
	pc.mtx.RLock()
	defer pc.mtx.RUnlock()

	c, ok := pc.cbChrMap[cbc]
	if !ok {
		return nil, fmt.Errorf("no characteristic with UUID=%v", cbc.UUID())
	}

	return c, nil
}

func (pc *profCache) findCbDsc(d *ble.Descriptor) (cbgo.Descriptor, error) {
	pc.mtx.RLock()
	defer pc.mtx.RUnlock()

	cbd, ok := pc.dscCbMap[d]
	if !ok {
		return cbd, fmt.Errorf("no CB descriptor with UUID=%v", d.UUID)
	}

	return cbd, nil
}

func (pc *profCache) findDsc(cbd cbgo.Descriptor) (*ble.Descriptor, error) {
	pc.mtx.RLock()
	defer pc.mtx.RUnlock()

	d, ok := pc.cbDscMap[cbd]
	if !ok {
		return nil, fmt.Errorf("no descriptor with UUID=%v", cbd.UUID())
	}

	return d, nil
}

// delSvc forgets a service with its characteristics and descriptors.
func (pc *profCache) delSvc(cbs cbgo.Service) {
	pc.mtx.Lock()
	defer pc.mtx.Unlock()

	for cbc, c := range pc.cbChrMap {
		if cbc.Service() != cbs {
			continue
		}
		for cbd, d := range pc.cbDscMap {
			if cbd.Characteristic() == cbc {
				delete(pc.cbDscMap, cbd)
				delete(pc.dscCbMap, d)
			}
		}
		delete(pc.cbChrMap, cbc)
		delete(pc.chrCbMap, c)
	}
	delete(pc.svcCbMap, pc.cbSvcMap[cbs])
	delete(pc.cbSvcMap, cbs)
}
