package dispatcher

import (
	"fmt"
	"log"
	"net/url"
	"path"

	"github.com/flosch/pongo2"
	"github.com/hashicorp/nomad/api"
	"github.com/tsocial/tessellate/storage/types"
	"github.com/tsocial/tessellate/tmpl"
)

const Papertrail = "Papertrail"

type client struct {
	cfg NomadConfig
}

type JobLog struct {
	Destination    string
	Aggregator     string
	PapertrailHost string
}

type NomadConfig struct {
	Address    string
	Username   string
	Password   string
	Datacenter string
	Image      string
	CPU        string
	Memory     string
	ConsulAddr string
	Log        *JobLog
}

func NewNomadClient(cfg NomadConfig) *client {
	return &client{cfg}
}

func MakeNomadJob(w string, c *client, j *types.Job) (string, error) {
	// Create a nomad job using go template
	var tmplStr = `
job "{{ job_name }}" {
  datacenters = ["{{ datacenter }}"]
  type        = "batch"

  group "{{ job_name }}" {
    count = 1

	restart {
      attempts = {{ attempts }}
    }

    task "apply_job" {
      driver = "docker"

      config {
        image = "{{ image }}"
        entrypoint = ["./tsl8", "-j", "{{ job_id }}", "-w", "{{ workspace_id }}", "-l", "{{ layout_id }}", "--consul-host", "{{ consul_addr }}"]

		logging {
		  type = "syslog"
		  config {
		    syslog-format  = "rfc3164"
		    syslog-address = "{{ log_destination }}"
		    tag            = "tsl8w-{{ job_name }}"
          }
        }
      }

      resources {
        cpu    = {{ cpu }}
        memory = {{ memory }}
      }
    }
  }
}
`
	cfg := pongo2.Context{
		"job_name":        w + "-" + j.LayoutId + "-" + j.Id,
		"job_id":          j.Id,
		"workspace_id":    w,
		"layout_id":       j.LayoutId,
		"datacenter":      c.cfg.Datacenter,
		"image":           c.cfg.Image,
		"cpu":             c.cfg.CPU,
		"memory":          c.cfg.Memory,
		"consul_addr":     c.cfg.ConsulAddr,
		"attempts":        j.Retry,
		"log_destination": c.cfg.Log.Destination,
	}

	if j.Dry {
		cfg["attempts"] = 0
	}

	return tmpl.Parse(tmplStr, cfg)

}

func (c *client) Dispatch(w string, j *types.Job) (string, error) {
	nomadJob, err := MakeNomadJob(w, c, j)
	if err != nil {
		log.Printf("error while job parsing: %+v", err)
		return "", err
	}

	log.Println(nomadJob)

	nConfig := api.DefaultConfig()
	nConfig.Address = c.cfg.Address

	if c.cfg.Username != "" {
		nConfig.HttpAuth = &api.HttpBasicAuth{
			Username: c.cfg.Username,
			Password: c.cfg.Password,
		}
	}

	cl, err := api.NewClient(nConfig)
	if err != nil {
		log.Printf("error while creating nomad client: %+v", err)
		return "", err
	}

	jobs := cl.Jobs()
	job, err := jobs.ParseHCL(nomadJob, true)
	if err != nil {
		log.Printf("error while parsing job hcl: %+v", err)
		return "", err
	}

	resp, _, err := jobs.Register(job, nil)
	if err != nil {
		log.Printf("error while registering nomad job: %+v", err)
		return "", err
	}

	log.Printf("successfully dispatched the job: %+v", resp)
	u, err := url.Parse(c.cfg.Address)
	if err != nil {
		log.Fatal(err)
	}

	var link string

	u.Path = path.Join(u.Path, "ui", "jobs", w+"-"+j.LayoutId+"-"+j.Id)
	link = u.String()

	if c.cfg.Log.Aggregator == Papertrail {
		var logUrl *url.URL
		jobFilter := fmt.Sprintf("tsl8w-%s-%s-%s", w, j.LayoutId, j.Id)

		if logUrl, err = url.Parse(fmt.Sprintf("%s/events?q=program:%s", c.cfg.Log.PapertrailHost, jobFilter)); err == nil {
			link = logUrl.String()
		} else {
			log.Printf("error generating papertrail url : %+v", err)
		}
	}

	return link, nil
}
