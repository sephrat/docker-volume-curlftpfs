package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

const socketAddress = "/run/docker/plugins/curlftpfs.sock"

type curlftpfsVolume struct {
	Address          string
	Credentials      string
	Uid		string
	Gid		string
	Umask		string

	Options []string

	HostMountpoint   string
	PluginMountpoint string
	connections      int
}

type curlftpfsDriver struct {
	sync.RWMutex

	root      string
	statePath string
	volumes   map[string]*curlftpfsVolume
}

func newCurlftpfsDriver(root string) (*curlftpfsDriver, error) {
	logrus.WithField("method", "new driver").Debug(root)

	d := &curlftpfsDriver{
		root:      filepath.Join(root, "volumes"),
		statePath: filepath.Join(root, "state", "curlftpfs-state.json"),
		volumes:   map[string]*curlftpfsVolume{},
	}

	data, err := ioutil.ReadFile(d.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			logrus.WithField("statePath", d.statePath).Debug("no state found")
		} else {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(data, &d.volumes); err != nil {
			return nil, err
		}
	}

	return d, nil
}

func (d *curlftpfsDriver) saveState() {
	data, err := json.Marshal(d.volumes)
	if err != nil {
		logrus.WithField("statePath", d.statePath).Error(err)
		return
	}

	if err := ioutil.WriteFile(d.statePath, data, 0644); err != nil {
		logrus.WithField("savestate", d.statePath).Error(err)
	}
}

func (d *curlftpfsDriver) Create(r *volume.CreateRequest) error {
	logrus.WithField("method", "create").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	v := &curlftpfsVolume{
		Uid: "1000",
		Gid: "1000",
		Umask: "0022",
	}

	for key, val := range r.Options {
		switch key {
		case "address":
			v.Address = val
		case "credentials":
			v.Credentials = val
		case "uid":
			v.Uid= val
		case "gid":
			v.Gid= val
		case "umask":
			v.Umask= val
		default:
			if val != "" {
				v.Options = append(v.Options, key+"="+val)
			} else {
				v.Options = append(v.Options, key)
			}
		}
	}

	if v.Address == "" {
		return logError("'address' option required")
	}

	hash:= md5.Sum([]byte(v.Address + v.Credentials))
	v.HostMountpoint = filepath.Join(d.root, fmt.Sprintf("%x", hash))
	v.PluginMountpoint = filepath.Join("/mnt/volumes/", fmt.Sprintf("%x", hash))

	d.volumes[r.Name] = v

	d.saveState()

	return nil
}

func (d *curlftpfsDriver) Remove(r *volume.RemoveRequest) error {
	logrus.WithField("method", "remove").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return logError("volume %s not found", r.Name)
	}

	if v.connections != 0 {
		return logError("volume %s is currently used by a container", r.Name)
	}
	if err := os.RemoveAll(v.HostMountpoint); err != nil {
		return logError(err.Error())
	}
	delete(d.volumes, r.Name)
	d.saveState()
	return nil
}

func (d *curlftpfsDriver) Path(r *volume.PathRequest) (*volume.PathResponse, error) {
	logrus.WithField("method", "path").Debugf("%#v", r)

	d.RLock()
	defer d.RUnlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return &volume.PathResponse{}, logError("volume %s not found", r.Name)
	}

	return &volume.PathResponse{Mountpoint: v.PluginMountpoint}, nil
}

func (d *curlftpfsDriver) Mount(r *volume.MountRequest) (*volume.MountResponse, error) {
	logrus.WithField("method", "mount").Debugf("%#v", r)
	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return &volume.MountResponse{}, logError("volume %s not found", r.Name)
	}

	if v.connections == 0 {
		fi, err := os.Lstat(v.HostMountpoint)
		if os.IsNotExist(err) {
			if err := os.MkdirAll(v.HostMountpoint, 0755); err != nil {
				return &volume.MountResponse{}, logError(err.Error())
			}
		} else if err != nil {
			return &volume.MountResponse{}, logError(err.Error())
		}

		if fi != nil && !fi.IsDir() {
			return &volume.MountResponse{}, logError("%v already exist and it's not a directory", v.HostMountpoint)
		}

		if err := d.mountVolume(v); err != nil {
			return &volume.MountResponse{}, logError(err.Error())
		}
	}

	v.connections++

	return &volume.MountResponse{Mountpoint: v.PluginMountpoint}, nil
}

func (d *curlftpfsDriver) Unmount(r *volume.UnmountRequest) error {
	logrus.WithField("method", "unmount").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()
	v, ok := d.volumes[r.Name]
	if !ok {
		return logError("volume %s not found", r.Name)
	}

	v.connections--

	if v.connections <= 0 {
		if err := d.unmountVolume(v.HostMountpoint); err != nil {
			return logError(err.Error())
		}
		v.connections = 0
	}

	return nil
}

func (d *curlftpfsDriver) Get(r *volume.GetRequest) (*volume.GetResponse, error) {
	logrus.WithField("method", "get").Debugf("%#v", r)

	d.Lock()
	defer d.Unlock()

	v, ok := d.volumes[r.Name]
	if !ok {
		return &volume.GetResponse{}, logError("volume %s not found", r.Name)
	}

	return &volume.GetResponse{Volume: &volume.Volume{Name: r.Name, Mountpoint: v.PluginMountpoint}}, nil
}

func (d *curlftpfsDriver) List() (*volume.ListResponse, error) {
	logrus.WithField("method", "list").Debugf("")

	d.Lock()
	defer d.Unlock()

	var vols []*volume.Volume
	for name, v := range d.volumes {
		vols = append(vols, &volume.Volume{Name: name, Mountpoint: v.PluginMountpoint})
	}
	return &volume.ListResponse{Volumes: vols}, nil
}

func (d *curlftpfsDriver) Capabilities() *volume.CapabilitiesResponse {
	logrus.WithField("method", "capabilities").Debugf("")

	return &volume.CapabilitiesResponse{Capabilities: volume.Capability{Scope: "local"}}
}

func (d *curlftpfsDriver) mountVolume(v *curlftpfsVolume) error {
	cmd := exec.Command("curlftpfs")
	cmd.Args = append(cmd.Args, "-o", "allow_other")
	if v.Credentials != "" {
		cmd.Args = append(cmd.Args, "-o", "user=" + v.Credentials)
	}
	if v.Uid != "" {
		cmd.Args = append(cmd.Args, "-o", "uid=" + v.Uid)
	}
	if v.Gid != "" {
		cmd.Args = append(cmd.Args, "-o", "gid=" + v.Gid)
	}
	if v.Umask!= "" {
		cmd.Args = append(cmd.Args, "-o", "umask=" + v.Umask)
	}
	for _, option := range v.Options {
               cmd.Args = append(cmd.Args, "-o", option)
        }
	cmd.Args = append(cmd.Args, v.Address, v.HostMountpoint)
	logrus.Debug(cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return logError("curlftpfs command execute failed: %v (%s)", err, output)
	}
	return nil
}

func (d *curlftpfsDriver) unmountVolume(target string) error {
	cmd := fmt.Sprintf("umount %s", target)
	logrus.Debug(cmd)
	return exec.Command("sh", "-c", cmd).Run()
}

func logError(format string, args ...interface{}) error {
	logrus.Errorf(format, args...)
	return fmt.Errorf(format, args...)
}

func main() {
	debug := os.Getenv("DEBUG")
	if ok, _ := strconv.ParseBool(debug); ok {
		logrus.SetLevel(logrus.DebugLevel)
	}

	d, err := newCurlftpfsDriver("/mnt")
	if err != nil {
		log.Fatal(err)
	}
	h := volume.NewHandler(d)
	logrus.Infof("listening on %s", socketAddress)
	logrus.Error(h.ServeUnix(socketAddress, 0))
}
