package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/flynn/flynn/Godeps/_workspace/src/gopkg.in/inconshreveable/log15.v2"
	"github.com/flynn/flynn/pkg/cluster"
)

var flynnHostLogs = map[string]string{
	"upstart-flynn-host.log": "/var/log/upstart/flynn-host.log",
	"tmp-flynn-host.log":     "/tmp/flynn-host.log",
}

var debugCmds = [][]string{
	{"ps", "faux"},
	{"ifconfig"},
	{"uname", "-a"},
	{"lsb_release", "-a"},
	{"date"},
	{"free", "-m"},
	{"dpkg-query", "-W", "-f", "${Package}: ${Version}\n", "libvirt-bin"},
	{os.Args[0], "version"},
	{"virsh", "-c", "lxc:///", "list"},
	{"virsh", "-c", "lxc:///", "net-list"},
}

func init() {
	Register("upload-debug-info", runUploadDebugInfo, `
usage: flynn-host upload-debug-info

Upload debug information to an anonymous gist`)
}

func runUploadDebugInfo() error {
	log := log15.New()
	log.Info("uploading logs and debug information to a private, anonymous gist")
	log.Info("this may take a while depending on the size of your logs")

	gist := &Gist{
		Description: "Flynn debug information",
		Public:      false,
		Files:       make(map[string]File),
	}

	log.Info("getting flynn-host logs")
	for name, filepath := range flynnHostLogs {
		if err := gist.AddLocalFile(name, filepath); err != nil && !os.IsNotExist(err) {
			log.Error(fmt.Sprintf("error getting flynn-host log %q", filepath), "err", err)
		}
	}

	log.Info("getting job logs")
	if err := captureJobs(gist); err != nil {
		log.Error("error getting job logs", "err", err)
	}

	log.Info("getting system information")
	var debugOutput string
	for _, cmd := range debugCmds {
		output, err := captureCmd(cmd[0], cmd[1:]...)
		if err != nil {
			log.Error(fmt.Sprintf("error capturing output of %q", strings.Join(cmd, " ")), "err", err)
			continue
		}
		debugOutput += fmt.Sprintln("===>", strings.Replace(strings.Join(cmd, " "), "\n", `\n`, -1), "\n", output)
	}
	gist.AddFile("0-debug-output.log", debugOutput)

	if err := gist.Upload(log); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("debug information uploaded to: %s", gist.URL))
	return nil
}

func captureCmd(name string, arg ...string) (string, error) {
	var buf bytes.Buffer
	c := exec.Command(name, arg...)
	c.Stdout = &buf
	if err := c.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func captureJobs(gist *Gist) error {
	client, err := cluster.NewClient()
	if err != nil {
		return err
	}

	jobs, err := jobList(client, true)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	printJobs(jobs, &buf)
	gist.AddFile("1-jobs.log", buf.String())

	for _, job := range jobs {
		var name string
		if app, ok := job.Job.Metadata["flynn-controller.app_name"]; ok {
			name += app + "-"
		}
		if typ, ok := job.Job.Metadata["flynn-controller.type"]; ok {
			name += typ + "-"
		}
		name += job.Job.ID + ".log"

		var content bytes.Buffer
		printJobDesc(&job, &content)
		fmt.Fprint(&content, "\n\n***** ***** ***** ***** ***** ***** ***** ***** ***** *****\n\n")
		getLog(job.HostID, job.Job.ID, client, false, true, &content, &content)

		gist.AddFile(name, content.String())
	}

	return nil
}
