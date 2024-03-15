package ssm

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/spreedly/confd/pkg/log"
)

type Client struct {
	client *ssm.SSM
}

func New() (*Client, error) {

	// Attempt to get AWS Region from ec2metadata. Should determine how to
	// shorten ec2metadata client timeout so it fails fast if not on EC2.
	metaSession, _ := session.NewSession()
	metaClient := ec2metadata.New(metaSession)
	var region string

	if os.Getenv("AWS_REGION") != "" {
		region = os.Getenv("AWS_REGION")
	} else {
		region, _ = metaClient.Region()
	}

	conf := aws.NewConfig().WithRegion(region)

	// Create a session to share configuration, and load external configuration.
	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
			Config:            *conf,
		},
	))

	log.Debug(fmt.Sprintf("Region: %s", aws.StringValue(sess.Config.Region)))

	// Fail early, if no credentials can be found
	_, err := sess.Config.Credentials.Get()
	if err != nil {
		return nil, err
	}

	var c *aws.Config
	if os.Getenv("SSM_LOCAL") != "" {
		log.Debug("SSM_LOCAL is set")
		endpoint := os.Getenv("SSM_ENDPOINT_URL")
		c = &aws.Config{
			Endpoint: &endpoint,
		}
	} else {
		c = nil
	}

	// Create the service's client with the session.
	svc := ssm.New(sess, c)
	return &Client{svc}, nil
}

// GetValues retrieves the values for the given keys from AWS SSM Parameter Store
func (c *Client) GetValues(keys []string) (map[string]string, error) {
	vars := make(map[string]string)
	var err error
	for _, key := range keys {
		log.Debug("Processing key=%s", key)
		var resp map[string]string
		resp, err = c.getParametersWithPrefix(key)
		if err != nil {
			return vars, err
		}
		if len(resp) == 0 {
			resp, err = c.getParameter(key)
			if err != nil && err.(awserr.Error).Code() != ssm.ErrCodeParameterNotFound {
				return vars, err
			}
		}
		for k, v := range resp {
			vars[k] = v
		}
	}
	return vars, nil
}

func (c *Client) getParametersWithPrefix(prefix string) (map[string]string, error) {
	var err error
	parameters := make(map[string]string)
	params := &ssm.GetParametersByPathInput{
		Path:           aws.String(prefix),
		Recursive:      aws.Bool(true),
		WithDecryption: aws.Bool(true),
	}
	err = c.client.GetParametersByPathPages(params,
		func(page *ssm.GetParametersByPathOutput, lastPage bool) bool {
			for _, p := range page.Parameters {
				parameters[*p.Name] = *p.Value
			}
			return !lastPage
		})
	return parameters, err
}

func (c *Client) getParameter(name string) (map[string]string, error) {
	parameters := make(map[string]string)
	params := &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	}
	resp, err := c.client.GetParameter(params)
	if err != nil {
		return parameters, err
	}
	parameters[*resp.Parameter.Name] = *resp.Parameter.Value
	return parameters, nil
}

// WatchPrefix is not implemented
func (c *Client) WatchPrefix(prefix string, keys []string, waitIndex uint64, stopChan chan bool) (uint64, error) {
	<-stopChan
	return 0, nil
}
