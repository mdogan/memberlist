package memberlist

import (
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
)

var bindLock sync.Mutex
var bindNum byte = 10

func getBindAddr() net.IP {
	bindLock.Lock()
	defer bindLock.Unlock()

	result := net.IPv4(127, 0, 0, bindNum)
	bindNum++
	if bindNum > 255 {
		bindNum = 10
	}

	return result
}

func testConfig() *Config {
	config := DefaultConfig()
	config.BindAddr = getBindAddr().String()
	config.Name = config.BindAddr
	return config
}

func yield() {
	time.Sleep(5 * time.Millisecond)
}

type MockDelegate struct {
	meta        []byte
	msgs        [][]byte
	broadcasts  [][]byte
	state       []byte
	remoteState []byte
}

func (m *MockDelegate) NodeMeta(limit int) []byte {
	return m.meta
}

func (m *MockDelegate) NotifyMsg(msg []byte) {
	m.msgs = append(m.msgs, msg)
}

func (m *MockDelegate) GetBroadcasts(overhead, limit int) [][]byte {
	b := m.broadcasts
	m.broadcasts = nil
	return b
}

func (m *MockDelegate) LocalState() []byte {
	return m.state
}

func (m *MockDelegate) MergeRemoteState(s []byte) {
	m.remoteState = s
}

func GetMemberlistDelegate(t *testing.T) (*Memberlist, *MockDelegate) {
	d := &MockDelegate{}

	c := testConfig()
	c.Delegate = d

	var m *Memberlist
	var err error
	for i := 0; i < 100; i++ {
		m, err = newMemberlist(c)
		if err == nil {
			return m, d
		}
		c.TCPPort++
		c.UDPPort++
	}
	t.Fatalf("failed to start: %v", err)
	return nil, nil
}

func GetMemberlist(t *testing.T) *Memberlist {
	c := testConfig()

	var m *Memberlist
	var err error
	for i := 0; i < 100; i++ {
		m, err = newMemberlist(c)
		if err == nil {
			return m
		}
		c.TCPPort++
		c.UDPPort++
	}
	t.Fatalf("failed to start: %v", err)
	return nil
}

func TestDefaultConfig_protocolVersion(t *testing.T) {
	c := DefaultConfig()
	if c.ProtocolVersion != ProtocolVersionMin {
		t.Fatalf("should be min: %d", c.ProtocolVersion)
	}
}

func TestCreate_protocolVersion(t *testing.T) {
	cases := []struct {
		version uint8
		err     bool
	}{
		{ProtocolVersionMin, false},
		{ProtocolVersionMax, false},
		// TODO(mitchellh): uncommon when we're over 0
		//{ProtocolVersionMin - 1, true},
		{ProtocolVersionMax + 1, true},
		{ProtocolVersionMax - 1, false},
	}

	for _, tc := range cases {
		c := DefaultConfig()
		c.BindAddr = getBindAddr().String()
		c.ProtocolVersion = tc.version
		m, err := Create(c)
		if tc.err && err == nil {
			t.Errorf("Should've failed with version: %d", tc.version)
		} else if !tc.err && err != nil {
			t.Errorf("Version '%d' error: %s", tc.version, err)
		}

		if err == nil {
			m.Shutdown()
		}
	}
}

func TestCreate(t *testing.T) {
	c := testConfig()
	m, err := Create(c)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer m.Shutdown()

	yield()

	members := m.Members()
	if len(members) != 1 {
		t.Fatalf("bad number of members")
	}

	if members[0].PMin != ProtocolVersionMin {
		t.Fatalf("bad: %#v", members[0])
	}

	if members[0].PMax != ProtocolVersionMax {
		t.Fatalf("bad: %#v", members[0])
	}
}

func TestMemberList_CreateShutdown(t *testing.T) {
	m := GetMemberlist(t)
	m.schedule()
	if err := m.Shutdown(); err != nil {
		t.Fatalf("failed to shutdown %v", err)
	}
}

func TestMemberList_Members(t *testing.T) {
	n1 := &Node{Name: "test"}
	n2 := &Node{Name: "test2"}
	n3 := &Node{Name: "test3"}

	m := &Memberlist{}
	nodes := []*nodeState{
		&nodeState{Node: *n1, State: stateAlive},
		&nodeState{Node: *n2, State: stateDead},
		&nodeState{Node: *n3, State: stateSuspect},
	}
	m.nodes = nodes

	members := m.Members()
	if !reflect.DeepEqual(members, []*Node{n1, n3}) {
		t.Fatalf("bad members")
	}
}

func TestMemberlist_Join(t *testing.T) {
	m1 := GetMemberlist(t)
	m1.setAlive()
	m1.schedule()
	defer m1.Shutdown()

	// Create a second node
	c := DefaultConfig()
	addr1 := getBindAddr()
	c.Name = addr1.String()
	c.BindAddr = addr1.String()
	c.UDPPort = m1.config.UDPPort
	c.TCPPort = m1.config.TCPPort

	m2, err := Create(c)
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}
	num, err := m2.Join([]string{m1.config.BindAddr})
	if num != 1 {
		t.Fatal("unexpected 1: %d", num)
	}
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}

	// Check the hosts
	if len(m2.Members()) != 2 {
		t.Fatalf("should have 2 nodes! %v", m2.Members())
	}
}

func TestMemberlist_Leave(t *testing.T) {
	m1 := GetMemberlist(t)
	m1.setAlive()
	m1.schedule()
	defer m1.Shutdown()

	// Create a second node
	c := DefaultConfig()
	addr1 := getBindAddr()
	c.Name = addr1.String()
	c.BindAddr = addr1.String()
	c.UDPPort = m1.config.UDPPort
	c.TCPPort = m1.config.TCPPort
	c.GossipInterval = time.Millisecond

	m2, err := Create(c)
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}
	num, err := m2.Join([]string{m1.config.BindAddr})
	if num != 1 {
		t.Fatal("unexpected 1: %d", num)
	}
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}

	// Check the hosts
	if len(m2.Members()) != 2 {
		t.Fatalf("should have 2 nodes! %v", m2.Members())
	}
	if len(m1.Members()) != 2 {
		t.Fatalf("should have 2 nodes! %v", m2.Members())
	}

	// Leave
	m1.Leave(time.Second)

	// Wait for leave
	time.Sleep(10 * time.Millisecond)

	// m1 should think dead
	if len(m1.Members()) != 1 {
		t.Fatalf("should have 1 node")
	}

	if len(m2.Members()) != 1 {
		t.Fatalf("should have 1 node")
	}
}

func TestMemberlist_JoinShutdown(t *testing.T) {
	m1 := GetMemberlist(t)
	m1.setAlive()
	m1.schedule()

	// Create a second node
	c := DefaultConfig()
	addr1 := getBindAddr()
	c.Name = addr1.String()
	c.BindAddr = addr1.String()
	c.UDPPort = m1.config.UDPPort
	c.TCPPort = m1.config.TCPPort
	c.ProbeInterval = time.Millisecond
	c.ProbeTimeout = 100 * time.Microsecond

	m2, err := Create(c)
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}
	num, err := m2.Join([]string{m1.config.BindAddr})
	if num != 1 {
		t.Fatal("unexpected 1: %d", num)
	}
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}

	// Check the hosts
	if len(m2.Members()) != 2 {
		t.Fatalf("should have 2 nodes! %v", m2.Members())
	}

	m1.Shutdown()

	time.Sleep(10 * time.Millisecond)

	if len(m2.Members()) != 1 {
		t.Fatalf("should have 1 nodes! %v", m2.Members())
	}
}

func TestMemberlist_delegateMeta(t *testing.T) {
	c1 := testConfig()
	c2 := testConfig()
	c1.Delegate = &MockDelegate{meta: []byte("web")}
	c2.Delegate = &MockDelegate{meta: []byte("lb")}

	m1, err := Create(c1)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer m1.Shutdown()

	m2, err := Create(c2)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer m2.Shutdown()

	_, err = m1.Join([]string{c2.BindAddr})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	yield()

	var roles map[string]string

	// Check the roles of members of m1
	m1m := m1.Members()
	if len(m1m) != 2 {
		t.Fatalf("bad: %#v", m1m)
	}

	roles = make(map[string]string)
	for _, m := range m1m {
		roles[m.Name] = string(m.Meta)
	}

	if r := roles[c1.Name]; r != "web" {
		t.Fatalf("bad role for %s: %s", c1.Name, r)
	}

	if r := roles[c2.Name]; r != "lb" {
		t.Fatalf("bad role for %s: %s", c2.Name, r)
	}

	// Check the roles of members of m2
	m2m := m2.Members()
	if len(m2m) != 2 {
		t.Fatalf("bad: %#v", m2m)
	}

	roles = make(map[string]string)
	for _, m := range m2m {
		roles[m.Name] = string(m.Meta)
	}

	if r := roles[c1.Name]; r != "web" {
		t.Fatalf("bad role for %s: %s", c1.Name, r)
	}

	if r := roles[c2.Name]; r != "lb" {
		t.Fatalf("bad role for %s: %s", c2.Name, r)
	}
}

func TestMemberlist_UserData(t *testing.T) {
	m1, d1 := GetMemberlistDelegate(t)
	d1.state = []byte("something")
	m1.setAlive()
	m1.schedule()
	defer m1.Shutdown()

	// Create a second delegate with things to send
	d2 := &MockDelegate{}
	d2.broadcasts = [][]byte{
		[]byte("test"),
		[]byte("foobar"),
	}
	d2.state = []byte("my state")

	// Create a second node
	c := DefaultConfig()
	addr1 := getBindAddr()
	c.Name = addr1.String()
	c.BindAddr = addr1.String()
	c.UDPPort = m1.config.UDPPort
	c.TCPPort = m1.config.TCPPort
	c.GossipInterval = time.Millisecond
	c.PushPullInterval = time.Millisecond
	c.Delegate = d2

	m2, err := Create(c)
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}
	num, err := m2.Join([]string{m1.config.BindAddr})
	if num != 1 {
		t.Fatal("unexpected 1: %d", num)
	}
	if err != nil {
		t.Fatal("unexpected err: %s", err)
	}
	defer m2.Shutdown()

	// Check the hosts
	if m2.NumMembers() != 2 {
		t.Fatalf("should have 2 nodes! %v", m2.Members())
	}

	// Wait for a little while
	time.Sleep(3 * time.Millisecond)

	// Ensure we got the messages
	if len(d1.msgs) != 2 {
		t.Fatalf("should have 2 messages!")
	}
	if !reflect.DeepEqual(d1.msgs[0], []byte("test")) {
		t.Fatalf("bad msg %v", d1.msgs[0])
	}
	if !reflect.DeepEqual(d1.msgs[1], []byte("foobar")) {
		t.Fatalf("bad msg %v", d1.msgs[1])
	}

	// Check the push/pull state
	if !reflect.DeepEqual(d1.remoteState, []byte("my state")) {
		t.Fatalf("bad state %s", d1.remoteState)
	}
	if !reflect.DeepEqual(d2.remoteState, []byte("something")) {
		t.Fatalf("bad state %s", d2.remoteState)
	}
}
