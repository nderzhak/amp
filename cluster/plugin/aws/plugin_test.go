package aws

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/session"
	cf "github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/satori/go.uuid"
)

var (
	keyName string
	region  string

	sess *session.Session
	svc  *cf.CloudFormation
)

// REQUIREMENTS:
// AWS credentials in path
// Environment variables set for the following:
//   KEYPAIR=<your-aws-key-pair>
//   REGION=<aws-region>
func init() {
	keyName = os.Getenv("KEYNAME")
	if keyName == "" {
		log.Fatal("KEYNAME environment variable not set")
	}
	region = os.Getenv("REGION")
	if region == "" {
		log.Fatal("REGION environment variable not set")
	}
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func setup() {
	sess = session.Must(session.NewSession())

	// Create the service's client with the session.
	svc = cf.New(sess,
		aws.NewConfig().WithRegion(region).WithLogLevel(aws.LogOff))
}

func teardown() {
	// TODO destroy test stacks
}

func TestCreate(t *testing.T) {

	stackName := fmt.Sprintf("%s-plugin-test-%s", keyName, uuid.NewV4())

	opts := &RequestOptions{
		TemplateURL: DefaultTemplateURL,
		Region:    region,
		StackName: stackName,
		OnFailure: "DELETE",
		Params: []string{
			fmt.Sprintf("KeyName=%s", keyName),
			fmt.Sprintf("ClusterSize=%s", "1"),
			fmt.Sprintf("ManagerSize=%s", "1"),
		},
		Sync: true,
	}

	log.Println("starting test...")

	// create stack
	// ============
	ctxCreate := context.Background()
	respCreate, err := CreateStack(ctxCreate, svc, opts, 20)
	if err != nil {
		t.Fatal(err)
	}
	log.Println(awsutil.StringValue(respCreate))
	log.Println("creating stack...")
	input := &cf.DescribeStacksInput{
		StackName: aws.String(opts.StackName),
	}
	if err := svc.WaitUntilStackCreateCompleteWithContext(ctxCreate, input); err != nil {
		t.Fatal(err)
	}
	log.Printf("stack created: %s\n", opts.StackName)

	// describe stack
	// ============
	ctxDescribe := context.Background()
	opts.Page = 1
	stackOutput, err := InfoStack(ctxDescribe, svc, opts)
	log.Println("printing stack output...")
	for _, so := range stackOutput {
		log.Printf("[%s] %s: %s\n", so.OutputKey, so.Description, so.OutputValue)
	}
	j, err := StackOutputToJSON(stackOutput)
	if err != nil {
		t.Fatal(err)
	}
	log.Println("printing stack output as json...")
	log.Println("-------------------------------------------------------")
	log.Println(j)
	log.Println("-------------------------------------------------------")
	// verify the json string at least starts with this:
	if !strings.HasPrefix(j, `{"output":[{"description":`) {
		t.Error("JSON response doesn't appear valid")
	}

	// update stack
	// ============
	// increase cluster from 1 to 2 workers
	opts.Params = append(opts.Params, fmt.Sprintf("ClusterSize=%s", "2"))
	ctxUpdate := context.Background()
	respUpdate, err := UpdateStack(ctxUpdate, svc, opts)
	if err != nil {
		t.Fatal(err)
	}
	log.Println(awsutil.StringValue(respUpdate))
	log.Println("updating stack...")
	if err := svc.WaitUntilStackUpdateCompleteWithContext(ctxUpdate, input); err != nil {
		t.Fatal(err)
	}
	log.Printf("stack updated: %s\n", opts.StackName)

	// delete stack
	// ============
	ctxDelete := context.Background()
	respDelete, err := DeleteStack(ctxDelete, svc, opts)
	if err != nil {
		t.Fatal(err)
	}
	log.Println(awsutil.StringValue(respDelete))
	log.Println("deleting stack...")
	if err := svc.WaitUntilStackDeleteCompleteWithContext(ctxDelete, input); err != nil {
		t.Fatal(err)
	}
	log.Printf("stack deleted: %s\n", opts.StackName)
}
