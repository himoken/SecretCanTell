package kenkenlab

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.himohimo-it.com/HIMO/himolab/app"
	"git.himohimo-it.com/HIMO/himolab/controllers/common"
	"git.himohimo-it.com/HIMO/himolab/kubeutil"
	"git.himohimo-it.com/HIMO/himolab/logs"
	"git.himohimo-it.com/HIMO/himolab/models"
	"git.himohimo-it.com/HIMO/himolab/utils"
	"github.com/astaxie/beego"
)

type FunctionInstanceController struct {
	beego.Controller
}

// @router / [get]
func (c *FunctionInstanceController) DisplayFunctionPage() {
	user := c.GetSession(common.UserSessionKey).(models.User)

	c.Data["labSettings"] = common.NewResourceManagerByModel(models.LabSetting{}).FetchUserAccessibleResources(user)
	c.Data["functionSettings"] = common.NewResourceManagerByModel(models.FunctionSetting{}).FetchUserAccessibleResources(user)
	c.Data["user"] = user
	c.Data["instanceTypeGroups"] = common.NewResourceManagerByModel(common.InstanceGroup{}).FetchUserAccessibleResources(user)
	c.Data["isAnonymous"] = app.GetConfig().User.AnonymousUserName == user.Name
	c.TplName = "kenkenlab/functioninstance.tpl"
}

// @router /detail/:id [get]
func (c *FunctionInstanceController) DisplayFunctionDetailPage() {
	user := c.GetSession(common.UserSessionKey).(models.User)
	instanceID := c.Ctx.Input.Param(":id")
	var instances []*models.FunctionSetting
	settingManager := models.NewModelManager(models.FunctionSetting{})
	settingManager.All(&instances)

	var targetInstance models.FunctionInstance
	instanceManager := models.NewModelManager(models.FunctionInstance{})
	err := instanceManager.FindOne(map[string]interface{}{
		"id":    instanceID,
		"owner": user.Name,
	}, &targetInstance)

	handler := new(models.FunctionHandlerManager).NewFunctionHandlerManager(targetInstance)

	if err != nil {
		c.Abort("404")
	} else {
		c.Data["targetInstance"] = targetInstance
		c.Data["functionContext"] = handler.GetFunctionContext()
		c.Data["instanceType"] = new(common.InstanceTypeManager).FindInstanceByName(targetInstance.InstanceTypeName)
		c.Data["deploymentLog"] = kubeutil.GetLog(user.Namespace, targetInstance.UUID)
		c.Data["internalEndpoints"] = kubeutil.GetInternalEndpoints(user.Namespace, targetInstance.UUID)
		c.Data["labSettings"] = common.NewResourceManagerByModel(models.LabSetting{}).FetchUserAccessibleResources(user)
		c.Data["user"] = user
		c.Data["isAnonymous"] = app.GetConfig().User.AnonymousUserName == user.Name
		c.TplName = "kenkenlab/functioninstancedetail.tpl"
	}

}

func buildFunctionStatus(user models.User) []*models.FunctionInstance {
	instanceManager := models.NewModelManager(models.FunctionInstance{})
	var functionInstances []*models.FunctionInstance
	instanceManager.FindInstances(map[string]interface{}{
		"owner": user.Name,
	}, &functionInstances)

	status := kubeutil.FetchFunctionStatus(user.Namespace)
	for _, instance := range functionInstances {
		if status, exist := status[instance.UUID]; exist {
			running := 0
			pending := 0
			url := ""

			for _, s := range status {
				if s.GetStatusID() == models.Running {
					running++
				} else {
					pending++
				}
				url = s.Endpoint
			}

			instance.RunningInstances = running
			instance.PendingInstances = pending
			originalURL := instance.URL
			var newURL string
			if instance.RunningInstances > 0 {
				settingManager := models.NewModelManager(models.FunctionSetting{})
				var functionSetting models.FunctionSetting
				settingManager.FindInstanceByName(instance.FunctionName, &functionSetting)
				if functionSetting.LoadBalancer != "" {
					newURL = fmt.Sprintf("%s%s/", functionSetting.LoadBalancer, kubeutil.GetFunctionIngressPath(instance))
				} else {
					newURL = url
				}
				if utils.CheckURLStatus(newURL) {
					instance.URL = newURL
				} else {
					instance.URL = ""
				}
				if originalURL != instance.URL {
					instanceManager.Update(instance)
				}
			}

		} else {

		}
	}
	return functionInstances
}

// @router /instances [get]
func (c *FunctionInstanceController) ListInstance() {
	user := c.GetSession(common.UserSessionKey).(models.User)
	c.Data["json"] = buildFunctionStatus(user)
	c.ServeJSON()
}

type FunctionRequest struct {
	FunctionContextType int    `form:"functionContextType"`
	InstanceName        string `form:"instanceName"`
	Trigger             string `form:"trigger"`
	FunctionName        string `form:"functionName"`
	InstanceTypeName    string `form:"instanceTypeName"`
	InstanceNumber      int    `form:"instanceNumber"`
	IngressPath         string `form:"ingressPath"`
	FunctionCode        string `form:"functionCode"`
	Requirement         string `form:"requirement"`
}

// @router /instances [post]
func (c *FunctionInstanceController) CreateInstance() {

	user := c.GetSession(common.UserSessionKey).(models.User)
	var request FunctionRequest
	c.ParseForm(&request)
	request.FunctionContextType = models.RawFunctionContext

	instanceManager := new(common.InstanceTypeManager)
	instanceTypeType := instanceManager.FindInstanceByName(request.InstanceTypeName)
	if instanceTypeType == nil {
		logs.GetLog().Info(fmt.Sprintf("Cannot find instance type:%s", request.InstanceTypeName))
		c.Data["json"] = utils.HTTPFailedJSON(fmt.Sprintf("Cannot find instance type:%s", request.InstanceTypeName))
		c.ServeJSON()
		return
	}

	settingManager := models.NewModelManager(models.FunctionSetting{})
	var functionSetting models.FunctionSetting
	err := settingManager.FindInstanceByName(request.FunctionName, &functionSetting)
	if err != nil {
		logs.GetLog().Info(fmt.Sprintf("Cannot find function name:%s", request.FunctionName))
		c.Data["json"] = utils.HTTPFailedJSON(fmt.Sprintf("Cannot find function name:%s", request.FunctionName))
		c.ServeJSON()
		return
	}

	resourceManager := common.NewResourceManagerByModel(models.FunctionSetting{})
	if err != nil || !resourceManager.CheckPermission(functionSetting.ID, user) {
		c.Data["json"] = utils.HTTPFailedJSON("No permission")
		c.ServeJSON()
		return
	}

	owner := user.Name
	namespace := user.Namespace
	//DNS-1123 subdomain must consist of lower case alphanumeric characters
	uuid := strings.Replace(
		strings.ToLower(fmt.Sprintf("function-%s-%s-%s", request.Trigger, owner, request.InstanceName)),
		".",
		"-",
		-1)
	instance := &models.FunctionInstance{
		UUID:                uuid,
		FunctionName:        request.FunctionName,
		Name:                request.InstanceName,
		InstanceTypeName:    request.InstanceTypeName,
		EphemeralStorage:    0,
		StorageScale:        "GiB",
		InstanceNumber:      request.InstanceNumber,
		Owner:               owner,
		URL:                 "",
		CreateAt:            time.Now().Unix(),
		Namespace:           namespace,
		IngressPath:         request.IngressPath,
		FunctionRef:         "",
		FunctionContextType: request.FunctionContextType,
		Trigger:             request.Trigger,
	}

	functionHandler := new(models.FunctionHandlerManager).NewFunctionHandlerManager(*instance)
	err = functionHandler.ApplyFunctionContext(models.FunctionContext{
		Code:        request.FunctionCode,
		Requirement: request.Requirement,
	})
	instance.FunctionRef = functionHandler.GetFunctionRef()

	if err != nil {
		c.Data["json"] = utils.HTTPFailedJSON("Apply function failed")
	} else {
		err = instance.Valid()
		if err != nil {
			c.Data["json"] = utils.HTTPFailedJSON(err.Error())
		} else {
			err = createKubeInstance(kubeutil.LoadFunctionInstanceSpec(instance, &functionSetting))
			if err != nil {
				c.Data["json"] = utils.HTTPFailedJSON("create kube instance error")
			} else {
				manager := models.NewModelManager(models.FunctionInstance{})
				if err := manager.Create(instance); err == nil {
					c.Data["json"] = utils.HTTPSuccessJSON()
				} else {
					c.Data["json"] = utils.HTTPFailedJSON("Insert error, maybe duplicated")
				}
			}
		}
	}

	c.ServeJSON()
}

// @router /instances/:id [delete]
func (c *FunctionInstanceController) DeleteInstance() {
	instanceID, _ := strconv.Atoi(c.Ctx.Input.Param(":id"))
	user := c.GetSession(common.UserSessionKey).(models.User)

	manager := models.NewModelManager(models.FunctionInstance{})
	var targetInstance models.FunctionInstance
	manager.FindOne(map[string]interface{}{
		"id":    instanceID,
		"owner": user.Name,
	}, &targetInstance)

	functionHandler := new(models.FunctionHandlerManager).NewFunctionHandlerManager(targetInstance)

	// Clear path
	err := functionHandler.DeleteFunctionContext()
	if err != nil {
		logs.GetLog().Error(err.Error())
	}
	// Delete db record
	err = manager.Delete(&targetInstance)

	if err == nil {
		//Delete kube instance
		err = deleteFunctionKubeInstance(&targetInstance)
		c.Data["json"] = utils.HTTPSuccessJSON()
	} else {
		c.Data["json"] = utils.HTTPFailedJSON(err.Error())
	}
	c.ServeJSON()
}

type UpdateFunctionRequest struct {
	InstanceNumber int    `form:"instanceNumber"`
	FunctionCode   string `form:"functionCode"`
	Requirement    string `form:"requirement"`
}

// @router /instances/:id [put]
func (c *FunctionInstanceController) UpdateInstance() {
	instanceID := c.Ctx.Input.Param(":id")
	user := c.GetSession(common.UserSessionKey).(models.User)
	var request UpdateFunctionRequest
	c.ParseForm(&request)
	var targetInstance models.FunctionInstance
	instanceManager := models.NewModelManager(models.FunctionInstance{})
	err := instanceManager.FindOne(map[string]interface{}{
		"id":    instanceID,
		"owner": user.Name,
	}, &targetInstance)
	logs.GetLog().Info(fmt.Sprintf("Update request: Id=%s, user=%s", instanceID, user.Name))
	if err == nil {
		newInstance := targetInstance
		err = newInstance.Valid()
		if err != nil {
			logs.GetLog().Error(err.Error())
			c.Data["json"] = utils.HTTPFailedJSON(err.Error())
		} else {

			functionHandler := new(models.FunctionHandlerManager).NewFunctionHandlerManager(targetInstance)
			oldContext := functionHandler.GetFunctionContext()
			newContext := models.FunctionContext{
				Code:        request.FunctionCode,
				Requirement: request.Requirement,
			}
			newInstance.InstanceNumber = request.InstanceNumber

			err = kubeutil.UpdateFunctionInstance(newInstance, oldContext, newContext)
			if err != nil {
				logs.GetLog().Error(err.Error())
				c.Data["json"] = utils.HTTPFailedJSON(err.Error())
			} else {
				functionHandler.ApplyFunctionContext(newContext)
				instanceManager.Update(&newInstance)
				c.Data["json"] = utils.HTTPSuccessJSON()
			}
		}
	} else {
		c.Data["json"] = utils.HTTPFailedJSON(err.Error())
	}
	c.ServeJSON()
}

// @router /instances/:id/restart [post]
func (c *FunctionInstanceController) RestartInstance() {
	instanceID := c.Ctx.Input.Param(":id")
	user := c.GetSession(common.UserSessionKey).(models.User)
	var targetInstance models.FunctionInstance
	instanceManager := models.NewModelManager(models.FunctionInstance{})
	err := instanceManager.FindOne(map[string]interface{}{
		"id":    instanceID,
		"owner": user.Name,
	}, &targetInstance)
	if err == nil {
		kubeutil.RestartFunctionInstance(user.Namespace, targetInstance.UUID)
		c.Data["json"] = utils.HTTPSuccessJSON()
	} else {
		c.Data["json"] = utils.HTTPFailedJSON(err.Error())
	}

	c.ServeJSON()
}

func deleteFunctionKubeInstance(instance *models.FunctionInstance) (err error) {

	err = kubeutil.DeleteFunctionService(instance)
	err = kubeutil.DeleteFunctionDeployment(instance)
	err = kubeutil.DeleteFunctionIngress(instance)
	return
}

// @router /testtool [get]
func (c *FunctionInstanceController) DisplayTestTool() {
	user := c.GetSession(common.UserSessionKey).(models.User)

	c.Data["labSettings"] = common.NewResourceManagerByModel(models.LabSetting{}).FetchUserAccessibleResources(user)
	c.Data["user"] = user
	c.Data["instanceTypeGroups"] = new(common.InstanceTypeManager).GetInstanceGroups()
	c.Data["isAnonymous"] = app.GetConfig().User.AnonymousUserName == user.Name
	c.TplName = "kenkenlab/testtool.tpl"
}

// @router /testtool [post]
func (c *FunctionInstanceController) ForwardHttpRequest() {

	endpoint := c.GetString("endpoint")
	method := c.GetString("method")
	jsonData := c.GetString("jsonData")
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr} //Skip https checking

	// Create request
	req, err := http.NewRequest(method, endpoint, bytes.NewBuffer([]byte(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		c.Data["json"] = utils.HTTPFailedJSON(err.Error())
	}
	// Fetch Request
	resp, err := client.Do(req)
	if err != nil {
		c.Data["json"] = utils.HTTPFailedJSON(err.Error())
	} else {
		defer resp.Body.Close()

		// Read Response Body
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			c.Data["json"] = utils.HTTPFailedJSON(err.Error())
		} else {
			c.Data["json"] = string(respBody)
		}
	}
	c.ServeJSON()
}
