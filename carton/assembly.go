/*
** Copyright [2013-2016] [Megam Systems]
**
** Licensed under the Apache License, Version 2.0 (the "License");
** you may not use this file except in compliance with the License.
** You may obtain a copy of the License at
**
** http://www.apache.org/licenses/LICENSE-2.0
**
** Unless required by applicable law or agreed to in writing, software
** distributed under the License is distributed on an "AS IS" BASIS,
** WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
** See the License for the specific language governing permissions and
** limitations under the License.
 */
package carton

import (
	"encoding/json"
	log "github.com/Sirupsen/logrus"
	ldb "github.com/megamsys/libgo/db"
	"github.com/megamsys/libgo/events"
	"github.com/megamsys/libgo/events/alerts"
	"github.com/megamsys/libgo/pairs"
	"github.com/megamsys/libgo/utils"
	constants "github.com/megamsys/libgo/utils"
	"github.com/megamsys/vertice/meta"
	"github.com/megamsys/vertice/provision"
	"gopkg.in/yaml.v2"
	"strings"
	"time"
)

const (
	ASSEMBLYBUCKET = "assembly"
	SSHKEY         = "sshkey"
	VNCPORT       = "vncport"
	VNCHOST       = "vnchost"
	VMID         =  "vmid"
)

type Policy struct {
	Name    string   `json:"name" cql:"name"`
	Type    string   `json:"type" cql:"type"`
	Members []string `json:"members" cql:"members"`
}

//An assembly comprises of various components.
type Ambly struct {
	Id         string   `json:"id" cql:"id"`
	OrgId      string   `json:"org_id" cql:"org_id"`
	AccountId  string   `json:"account_id" cql:"account_id"`
	Name       string   `json:"name" cql:"name"`
	JsonClaz   string   `json:"json_claz" cql:"json_claz"`
	Tosca      string   `json:"tosca_type" cql:"tosca_type"`
	Inputs     []string `json:"inputs" cql:"inputs"`
	Outputs    []string `json:"outputs" cql:"outputs"`
	Policies   []string `json:"policies" cql:"policies"`
	Status     string   `json:"status" cql:"status"`
	CreatedAt  string   `json:"created_at" cql:"created_at"`
	Components []string `json:"components" cql:"components"`
}

type Assembly struct {
	Id         string          `json:"id" cql:"id"`
	OrgId      string          `json:"org_id" cql:"org_id"`
	AccountId  string          `json:"account_id" cql:"account_id"`
	Name       string          `json:"name" cql:"name"`
	JsonClaz   string          `json:"json_claz" cql:"json_claz"`
	Tosca      string          `json:"tosca_type" cql:"tosca_type"`
	Inputs     pairs.JsonPairs `json:"inputs" cql:"inputs"`
	Outputs    pairs.JsonPairs `json:"outputs" cql:"outputs"`
	Policies   []*Policy       `json:"policies" cql:"policies"`
	Status     string          `json:"status" cql:"status"`
	CreatedAt  string          `json:"created_at" cql:"created_at"`
	Components map[string]*Component
}

func (a *Assembly) String() string {
	if d, err := yaml.Marshal(a); err != nil {
		return err.Error()
	} else {
		return string(d)
	}
}

//Assembly into a carton.
//a carton comprises of self contained boxes
func mkCarton(aies string, ay string) (*Carton, error) {
	a, err := get(ay)
	if err != nil {
		return nil, err
	}

	b, err := a.mkBoxes(aies)
	if err != nil {
		return nil, err
	}

	c := &Carton{
		Id:           ay,   //assembly id
		CartonsId:    aies, //assemblies id
		Name:         a.Name,
		Tosca:        a.Tosca,
		ImageVersion: a.imageVersion(),
		DomainName:   a.domain(),
		Compute:      a.newCompute(),
		SSH:          a.newSSH(),
		Provider:     a.provider(),
		PublicIp:     a.publicIp(),
		//VncHost:      a.vncHost(),
		//VncPort:      a.vncPort(),
		Boxes:        &b,
		Status:       utils.Status(a.Status),
	}
	return c, nil
}

//lets make boxes with components to be mutated later or, and the required
//information for a launch.
//A "colored component" externalized with what we need.
func (a *Assembly) mkBoxes(aies string) ([]provision.Box, error) {
	newBoxs := make([]provision.Box, 0, len(a.Components))
	for _, comp := range a.Components {
		if len(strings.TrimSpace(comp.Id)) > 1 {
			if b, err := comp.mkBox(); err != nil {
				return nil, err
			} else {
				b.CartonId = a.Id
				b.CartonsId = aies
				b.CartonName = a.Name
				if len(strings.TrimSpace(b.Provider)) <= 0 {
					b.Provider = a.provider()
				}
				if len(strings.TrimSpace(b.PublicIp)) <= 0 {
					b.PublicIp = a.publicIp()
				}
				if b.Repo.IsEnabled() {
					b.Repo.Hook.CartonId = a.Id //this is screwy, why do we need it.
					b.Repo.Hook.BoxId = comp.Id
				}
				b.Compute = a.newCompute()
				b.SSH = a.newSSH()
				b.Status = utils.Status(a.Status)
				newBoxs = append(newBoxs, b)
			}
		}
	}
	return newBoxs, nil
}

func getBig(id string) (*Ambly, error) {
	a := &Ambly{}
	ops := ldb.Options{
		TableName:   ASSEMBLYBUCKET,
		Pks:         []string{"Id"},
		Ccms:        []string{},
		Hosts:       meta.MC.Scylla,
		Keyspace:    meta.MC.ScyllaKeyspace,
		PksClauses:  map[string]interface{}{"Id": id},
		CcmsClauses: make(map[string]interface{}),
	}
	if err := ldb.Fetchdb(ops, a); err != nil {
		return nil, err
	}
	return a, nil
}

//Temporary hack to create an assembly from its id.
//This is used by SetStatus.
//We need add a Notifier interface duck typed by Box and Carton ?
func NewAssembly(id string) (*Assembly, error) {
	return get(id)
}

func NewAmbly(id string) (*Ambly, error) {
	return getBig(id)
}

func NewAssemblyToCart(aies string, ay string) (*Carton, error) {
	return mkCarton(aies, ay)
}

func (a *Ambly) SetStatus(status utils.Status) error {
	js := a.getInputs()
	LastStatusUpdate := time.Now().Local().Format(time.RFC822)
	m := make(map[string][]string, 2)
	m["lastsuccessstatusupdate"] = []string{LastStatusUpdate}
	m["status"] = []string{status.String()}
	js.NukeAndSet(m) //just nuke the matching output key:
	a.Status = status.String()

	update_fields := make(map[string]interface{})
	update_fields["Inputs"] = js.ToString()
	update_fields["Status"] = status.String()
	ops := ldb.Options{
		TableName:   ASSEMBLYBUCKET,
		Pks:         []string{"id"},
		Ccms:        []string{"org_id"},
		Hosts:       meta.MC.Scylla,
		Keyspace:    meta.MC.ScyllaKeyspace,
		PksClauses:  map[string]interface{}{"id": a.Id},
		CcmsClauses: map[string]interface{}{"org_id": a.OrgId},
	}
	if err := ldb.Updatedb(ops, update_fields); err != nil {
		return err
	}
	return a.trigger_event(status)

}

func (a *Ambly) trigger_event(status utils.Status) error {
	mi := make(map[string]string)

	js := make(pairs.JsonPairs, 0)
	m := make(map[string][]string, 2)
	m["status"] = []string{status.String()}
	m["description"] = []string{status.Description(a.Name)}
	js.NukeAndSet(m) //just nuke the matching output key:

	mi[constants.ASSEMBLY_ID] = a.Id
	mi[constants.ACCOUNT_ID] = a.AccountId
	mi[constants.EVENT_TYPE] = status.Event_type()

	newEvent := events.NewMulti(
		[]*events.Event{
			&events.Event{
				AccountsId:  a.AccountId,
				EventAction: alerts.STATUS,
				EventType:   constants.EventUser,
				EventData:   alerts.EventData{M: mi, D: js.ToString()},
				Timestamp:   time.Now().Local(),
			},
		})
	return newEvent.Write()
}

//update outputs in scylla, nuke the matching keys available
func (a *Ambly) NukeAndSetOutputs(m map[string][]string) error {
	if len(m) > 0 {
		log.Debugf("nuke and set outputs in scylla [%s]", m)
		js := a.getOutputs()
		js.NukeAndSet(m) //just nuke the matching output key:
		update_fields := make(map[string]interface{})
		update_fields["Outputs"] = js.ToString()
		ops := ldb.Options{
			TableName:   ASSEMBLYBUCKET,
			Pks:         []string{"id"},
			Ccms:        []string{"org_id"},
			Hosts:       meta.MC.Scylla,
			Keyspace:    meta.MC.ScyllaKeyspace,
			PksClauses:  map[string]interface{}{"id": a.Id},
			CcmsClauses: map[string]interface{}{"org_id": a.OrgId},
		}
		if err := ldb.Updatedb(ops, update_fields); err != nil {
			return err
		}
	} else {
		return provision.ErrNoOutputsFound
	}
	return nil
}

func (c *Assembly) Delete(asmid string) {
	ops := ldb.Options{
		TableName:   ASSEMBLYBUCKET,
		Pks:         []string{"id"},
		Ccms:        []string{"org_id"},
		Hosts:       meta.MC.Scylla,
		Keyspace:    meta.MC.ScyllaKeyspace,
		PksClauses:  map[string]interface{}{"id": asmid},
		CcmsClauses: map[string]interface{}{"org_id": c.OrgId},
	}
	if err := ldb.Deletedb(ops, Ambly{}); err != nil {
		return
	}
}

//get the assembly and its children (component). we only store the
//componentid, hence you see that we have a components map to cater to that need.
func get(id string) (*Assembly, error) {
	a := &Ambly{}
	ops := ldb.Options{
		TableName:   ASSEMBLYBUCKET,
		Pks:         []string{"Id"},
		Ccms:        []string{},
		Hosts:       meta.MC.Scylla,
		Keyspace:    meta.MC.ScyllaKeyspace,
		PksClauses:  map[string]interface{}{"Id": id},
		CcmsClauses: make(map[string]interface{}),
	}
	if err := ldb.Fetchdb(ops, a); err != nil {
		return nil, err
	}
	asm, _ := a.dig()
	return &asm, nil
}

func (a *Ambly) dig() (Assembly, error) {
	asm := Assembly{}
	asm.Id = a.Id
	asm.AccountId = a.AccountId
	asm.Name = a.Name
	asm.Tosca = a.Tosca
	asm.JsonClaz = a.JsonClaz
	asm.Inputs = a.getInputs()
	asm.Outputs = a.getOutputs()
	asm.Policies = a.getPolicies()
	asm.Status = a.Status
	asm.CreatedAt = a.CreatedAt
	asm.Components = make(map[string]*Component)
	for _, cid := range a.Components {
		if len(strings.TrimSpace(cid)) > 1 {
			if comp, err := NewComponent(cid); err != nil {
				log.Errorf("Failed to get component %s from scylla: %s.", cid, err.Error())
				return asm, err
			} else {
				asm.Components[cid] = comp
			}
		}
	}
	return asm, nil
}

func (a *Assembly) sshkey() string {
	return a.Inputs.Match(SSHKEY)
}

func (a *Assembly) domain() string {
	return a.Inputs.Match(DOMAIN)
}

func (a *Assembly) provider() string {
	return a.Inputs.Match(utils.PROVIDER)
}

func (a *Assembly) publicIp() string {
	return a.Outputs.Match(PUBLICIPV4)
}
func (a *Assembly) vncHost() string {
	return a.Outputs.Match(VNCHOST)
}
func (a *Assembly) vncPort() string {
	return a.Outputs.Match(VNCPORT)
}
func (a *Assembly) vmId() string {
	return a.Outputs.Match(VMID)
}
func (a *Assembly) imageVersion() string {
	return a.Inputs.Match(IMAGE_VERSION)
}

func (a *Assembly) newCompute() provision.BoxCompute {
	return provision.BoxCompute{
		Cpushare: a.getCpushare(),
		Memory:   a.getMemory(),
		Swap:     a.getSwap(),
		HDD:      a.getHDD(),
	}
}

func (a *Assembly) newSSH() provision.BoxSSH {
	return provision.BoxSSH{
		User:   meta.MC.User,
		Prefix: a.sshkey(),
	}
}

func (a *Assembly) getCpushare() string {
	return a.Inputs.Match(provision.CPU)
}

func (a *Assembly) getMemory() string {
	return a.Inputs.Match(provision.RAM)
}

func (a *Assembly) getSwap() string {
	return ""
}

//The default HDD is 10. we should configure it in the vertice.conf
func (a *Assembly) getHDD() string {
	if len(strings.TrimSpace(a.Inputs.Match(provision.HDD))) <= 0 {
		return "10"
	}
	return a.Inputs.Match(provision.HDD)
}

func (a *Ambly) getInputs() pairs.JsonPairs {
	keys := make([]*pairs.JsonPair, 0)
	for _, in := range a.Inputs {
		inputs := pairs.JsonPair{}
		parseStringToStruct(in, &inputs)
		keys = append(keys, &inputs)
	}
	return keys
}

func (a *Ambly) getOutputs() pairs.JsonPairs {
	keys := make([]*pairs.JsonPair, 0)
	for _, in := range a.Outputs {
		outputs := pairs.JsonPair{}
		parseStringToStruct(in, &outputs)
		keys = append(keys, &outputs)
	}
	return keys
}

func (a *Ambly) getPolicies() []*Policy {
	keys := make([]*Policy, 0)
	for _, in := range a.Policies {
		p := Policy{}
		parseStringToStruct(in, &p)
		keys = append(keys, &p)
	}
	return keys
}

func parseStringToStruct(str string, data interface{}) error {
	if err := json.Unmarshal([]byte(str), data); err != nil {
		return err
	}
	return nil
}
