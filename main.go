package main

import (
	"os"
	"fmt"
	"strings"
	"reflect"
	flag "github.com/ogier/pflag"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/mitchellh/go-homedir"
	"github.com/vaughan0/go-ini"
)

const (
	programName = "ec2ls"
	programVersion = "0.0.3"
)

var defaultAttributes = [...]string{
	"InstanceId",
	"Name",
	"InstanceType",
	"AvailabilityZone",
	"State",
	"PrivateIpAddress",
	"PublicIpAddress",
}

func name(i *ec2.Instance) string {
	tags := i.Tags
	for _, tag := range tags {
		if "Name" == *tag.Key {
			return *tag.Value
		}
	}
	return ""
}

func state(i *ec2.Instance) string {
	return *i.State.Name
}

func availabilityZone(i *ec2.Instance) string {
	return *i.Placement.AvailabilityZone
}

var getter = map[string](func(*ec2.Instance)string){
	"Name":  name,
	"State":  state,
	"AvailabilityZone":  availabilityZone,
}

func createCredentials(profileName *string) *credentials.Credentials {
	fmt.Println("Using profile:", *profileName)
	profileFilepath, err := homedir.Expand("~/.aws/credentials")
	if err != nil {
		panic(err)
	}

	file, err := ini.LoadFile(profileFilepath)
	if err != nil {
		panic(err)
	}

	sourceProfile, ok := file.Get(*profileName, "source_profile")
	if ok {
		// Using AssumeRole
		roleArn, _ := file.Get(*profileName, "role_arn")
		roleSessionName := "ec2ls"
		creds := credentials.NewSharedCredentials(profileFilepath, sourceProfile)
		svc := sts.New(session.New(), &aws.Config{Credentials: creds})
		result, err := svc.AssumeRole(&sts.AssumeRoleInput{
			RoleArn: &roleArn,
			RoleSessionName: &roleSessionName,
		})
		if err != nil {
			panic(err)
		}

		tmpCreds := result.Credentials
		return credentials.NewStaticCredentials(
			*tmpCreds.AccessKeyId,
			*tmpCreds.SecretAccessKey,
			*tmpCreds.SessionToken)
	}
	return credentials.NewSharedCredentials(profileFilepath, *profileName)
}

func main() {
	config := aws.Config{}
	profileName := flag.StringP("profile", "p", "", "AWS profile")
	version := flag.BoolP("version", "v", false, "Show version")
	flag.Parse()

	if *version {
		fmt.Println(programName, programVersion)
		os.Exit(0)
	}

	if len(*profileName) > 0 {
		config.Credentials = createCredentials(profileName)
	}

	// Create an EC2 service object in the "us-west-2" region
	// Note that you can also configure your region globally by
	// exporting the AWS_REGION environment variable
	svc := ec2.New(session.New(), &config)

	// Call the DescribeInstances Operation
	resp, err := svc.DescribeInstances(nil)
	if err != nil {
		panic(err)
	}

	// resp has all of the response data, pull out instance IDs:
	for idx := range resp.Reservations {
		for _, inst := range resp.Reservations[idx].Instances {
			attrs := make([]string, 0, 10)
			for _, attr := range defaultAttributes {
				v := ""
				f, ok := getter[attr]
				if ok {
					v = f(inst)
				} else {
					field := reflect.ValueOf(*inst).FieldByName(attr)
					if !field.IsNil() {
						v = fmt.Sprintf("%v", field.Elem().Interface())
					}
				}
				attrs = append(attrs, v)
			}
			fmt.Println(strings.Join(attrs, "\t"))
		}
	}
}

