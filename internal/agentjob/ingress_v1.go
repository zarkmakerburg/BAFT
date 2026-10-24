package agentjob

import (
	"errors"
	"net"
	"strconv"
)

var operationalIngressFields = []string{
	"ingress_protocol", "operation_id", "lease_id", "plan_sha256",
	"config_sha256", "listen_endpoint", "transport",
}

// validateIngressV1 rejects mixed legacy/v1 signed params without treating
// schema admission as permission to alter a host. Executor remains disabled.
func validateIngressV1(j Job) error {
	if j.Action != ActionPublicIngressApply && j.Action != ActionPublicIngressRetire {
		return nil
	}
	marked := j.Params["ingress_protocol"] == "1"
	if !marked {
		for _, k := range operationalIngressFields {
			if _, exists := j.Params[k]; exists {
				return errors.New("mixed legacy and operational ingress fields")
			}
		}
		return nil
	}
	for _, k := range operationalIngressFields {
		if j.Params[k] == "" {
			return errors.New("operational ingress field is required: " + k)
		}
	}
	if j.Params["generation"] != "1" {
		return errors.New("operational ingress generation transition unsupported")
	}
	return validateIngressV1Endpoint(j)
}

func validateIngressV1Endpoint(j Job) error {
	endpoint := j.Params["listen_endpoint"]
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return errors.New("invalid ingress listen endpoint")
	}
	ip := net.ParseIP(host)
	port, err := strconv.Atoi(portText)
	if err != nil || ip == nil || port < 1 || port > 65535 {
		return errors.New("invalid ingress port")
	}
	if endpoint != net.JoinHostPort(ip.String(), strconv.Itoa(port)) || j.Params["public_port"] != strconv.Itoa(port) || port == 22 {
		return errors.New("operational ingress canonical port mismatch")
	}
	return nil
}
