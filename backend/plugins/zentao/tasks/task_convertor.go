/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasks

import (
	"reflect"
	"strconv"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer"
	"github.com/apache/incubator-devlake/core/models/domainlayer/didgen"
	"github.com/apache/incubator-devlake/core/models/domainlayer/ticket"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/zentao/models"
)

var _ plugin.SubTaskEntryPoint = ConvertTask

var ConvertTaskMeta = plugin.SubTaskMeta{
	Name:             "convertTask",
	EntryPoint:       ConvertTask,
	EnabledByDefault: true,
	Description:      "convert Zentao task",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
}

func ConvertTask(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*ZentaoTaskData)
	db := taskCtx.GetDal()
	storyIdGen := didgen.NewDomainIdGenerator(&models.ZentaoStory{})
	boardIdGen := didgen.NewDomainIdGenerator(&models.ZentaoExecution{})
	taskIdGen := didgen.NewDomainIdGenerator(&models.ZentaoTask{})
	cursor, err := db.Cursor(
		dal.From(&models.ZentaoTask{}),
		dal.Where(`project = ? and connection_id = ?`, data.Options.ProjectId, data.Options.ConnectionId),
	)
	if err != nil {
		return err
	}
	defer cursor.Close()
	convertor, err := api.NewDataConverter(api.DataConverterArgs{
		InputRowType: reflect.TypeOf(models.ZentaoTask{}),
		Input:        cursor,
		RawDataSubTaskArgs: api.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Params: ZentaoApiParams{
				ConnectionId: data.Options.ConnectionId,
				ProductId:    data.Options.ProductId,
				ProjectId:    data.Options.ProjectId,
			},
			Table: RAW_TASK_TABLE,
		},
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			toolEntity := inputRow.(*models.ZentaoTask)

			domainEntity := &ticket.Issue{
				DomainEntity: domainlayer.DomainEntity{
					Id: taskIdGen.Generate(toolEntity.ConnectionId, toolEntity.ID),
				},
				IssueKey:        strconv.FormatInt(toolEntity.ID, 10),
				Title:           toolEntity.Name,
				Description:     toolEntity.Description,
				Type:            ticket.TASK,
				OriginalType:    toolEntity.Type + "." + toolEntity.Mode,
				OriginalStatus:  toolEntity.Status,
				ResolutionDate:  toolEntity.ClosedDate.ToNullableTime(),
				CreatedDate:     toolEntity.OpenedDate.ToNullableTime(),
				UpdatedDate:     toolEntity.LastEditedDate.ToNullableTime(),
				ParentIssueId:   storyIdGen.Generate(data.Options.ConnectionId, toolEntity.Parent),
				Priority:        getPriority(toolEntity.Pri),
				CreatorId:       strconv.FormatInt(toolEntity.OpenedById, 10),
				CreatorName:     toolEntity.OpenedByName,
				AssigneeId:      strconv.FormatInt(toolEntity.AssignedToId, 10),
				AssigneeName:    toolEntity.AssignedToName,
				Url:             toolEntity.Url,
				OriginalProject: getOriginalProject(data),
				Status:          toolEntity.StdStatus,
			}

			if toolEntity.ClosedDate != nil {
				domainEntity.LeadTimeMinutes = int64(toolEntity.ClosedDate.ToNullableTime().Sub(toolEntity.OpenedDate.ToTime()).Minutes())
			}
			var results []interface{}
			if domainEntity.AssigneeId != "" {
				issueAssignee := &ticket.IssueAssignee{
					IssueId:      domainEntity.Id,
					AssigneeId:   domainEntity.AssigneeId,
					AssigneeName: domainEntity.AssigneeName,
				}
				results = append(results, issueAssignee)
			}
			domainBoardIssue := &ticket.BoardIssue{
				BoardId: boardIdGen.Generate(data.Options.ConnectionId, toolEntity.Execution),
				IssueId: domainEntity.Id,
			}
			results = append(results, domainEntity, domainBoardIssue)
			return results, nil
		},
	})

	if err != nil {
		return err
	}

	return convertor.Execute()
}
