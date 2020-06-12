package node

import (
	"bytes"
	"fmt"
	"github.com/innobead/kubefire/pkg/config"
	"github.com/innobead/kubefire/pkg/data"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"html/template"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

const (
	RunCmd    = "ignite run {{.Image}} --name={{.Name}} --ssh --kernel-image={{.KernelImage}} --cpus={{.Cpus}} --memory={{.Memory}} --size={{.DiskSize}}"
	DeleteCmd = "ignite rm {{.Name}} --force"
)

type IgniteNodeManager struct {
}

func NewIgniteNodeManager() *IgniteNodeManager {
	return &IgniteNodeManager{}
}

func (i *IgniteNodeManager) CreateNodes(nodeType Type, node *config.Node) error {
	logrus.Infof("Creating nodes of cluster (%s)", node.Cluster.Name)

	tmp, err := template.New("create").Parse(RunCmd)
	if err != nil {
		return errors.WithStack(err)
	}

	var wgCreateNode sync.WaitGroup

	for i := 1; i <= node.Count; i++ {
		tmpBuffer := &bytes.Buffer{}

		node := &struct {
			Name        string
			Image       string
			KernelImage string
			KernelArgs  string
			Cpus        int
			Memory      string
			DiskSize    string
		}{
			Name:        fmt.Sprintf("%s-%s-%s", node.Cluster.Name, nodeType, strconv.Itoa(i)),
			Image:       node.Cluster.Image,
			KernelImage: node.Cluster.KernelImage,
			KernelArgs:  node.Cluster.KernelArgs,
			Cpus:        node.Cpus,
			Memory:      node.Memory,
			DiskSize:    node.DiskSize,
		}

		if err := tmp.Execute(tmpBuffer, node); err != nil {
			return errors.WithStack(err)
		}

		cmd := exec.Command("sudo", strings.Split(tmpBuffer.String(), " ")...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		logrus.Infof("Creating node (%s)", node.Name)

		err := cmd.Start()
		if err != nil {
			return errors.WithStack(err)
		}

		wgCreateNode.Add(1)

		go func(name string) {
			defer wgCreateNode.Done()

			if err := cmd.Wait(); err != nil {
				logrus.WithError(err).Errorf("failed to create node (%s)", name)
			}
		}(node.Name)
	}

	wgCreateNode.Wait()

	return nil
}

func (i *IgniteNodeManager) DeleteNodes(nodeType Type, node *config.Node) error {
	logrus.Infof("Deleting nodes of type (%s)", nodeType)

	for j := 1; j <= node.Count; j++ {
		name := fmt.Sprintf("%s-%s-%s", node.Cluster.Name, nodeType, strconv.Itoa(j))
		if err := i.DeleteNode(name); err != nil {
			return err
		}
	}

	return nil
}

func (i *IgniteNodeManager) DeleteNode(name string) error {
	logrus.Infof("Deleting node (%s)", name)

	tmp, err := template.New("delete").Parse(DeleteCmd)
	if err != nil {
		return errors.WithStack(err)
	}

	tmpBuffer := &bytes.Buffer{}

	c := &struct {
		Name string
	}{
		Name: name,
	}
	if err := tmp.Execute(tmpBuffer, c); err != nil {
		return errors.WithStack(err)
	}

	cmd := exec.Command("sudo", strings.Split(tmpBuffer.String(), " ")...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (i *IgniteNodeManager) GetNode(name string) (*data.Node, error) {
	logrus.Debugf("Getting node (%s)", name)

	cmdArgs := strings.Split(fmt.Sprintf("ignite ps --all -f {{.ObjectMeta.Name}}=%s", name), " ")
	cmd := exec.Command("sudo", cmdArgs...)

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	node := &data.Node{
		Name:   name,
		Spec:   config.Node{},
		Status: data.NodeStatus{},
	}

	nodeValueFilters := map[interface{}]map[string]string{
		&node.Spec: {
			"{{.Spec.CPUs}}":     "Cpus",
			"{{.Spec.Memory}}":   "Memory",
			"{{.Spec.DiskSize}}": "DiskSize",
		},
		&node.Status: {
			"{{.Status.Running}}": "Running",
		},
	}

	for v, filters := range nodeValueFilters {
		nodeValue := reflect.ValueOf(v).Elem()

		for filter, field := range filters {
			newCmdArgs := cmdArgs
			newCmdArgs = append(newCmdArgs, "-t "+filter)

			cmd := exec.Command("sudo", newCmdArgs...)
			cmd.Stderr = os.Stderr

			output, err := cmd.Output()
			if err != nil {
				return nil, errors.WithStack(err)
			}

			fieldValue := strings.TrimSuffix(strings.TrimSpace(string(output)), "\n")

			f := nodeValue.FieldByName(field)
			switch f.Kind() {
			case reflect.String:
				f.SetString(fieldValue)

			case reflect.Int:
				if i, err := strconv.ParseInt(fieldValue, 10, 64); err == nil {
					f.SetInt(i)
				}

			case reflect.Bool:
				if b, err := strconv.ParseBool(fieldValue); err == nil {
					f.SetBool(b)
				}
			}
		}
	}

	return node, nil
}

func (i *IgniteNodeManager) ListNodes(clusterName string) ([]*data.Node, error) {
	logrus.Debugf("Listing nodes of cluster (%s)", clusterName)

	cmdArgs := strings.Split("ignite ps --all", " ")

	if clusterName != "" {
		cmdArgs = append(
			cmdArgs,
			"-f",
			fmt.Sprintf("{{.ObjectMeta.Name}}=~%s", clusterName),
			"-t",
			"{{.ObjectMeta.Name}}",
		)
	}

	cmd := exec.Command("sudo", cmdArgs...)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var nodes []*data.Node

	if len(output) > 0 {
		names := strings.Split(strings.TrimSpace(string(output)), "\n")

		for _, n := range names {
			node, err := i.GetNode(n)
			if err != nil {
				return nil, err
			}

			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}
