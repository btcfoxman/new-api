package model

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func createTaskQueryUser(t *testing.T, username string) *User {
	t.Helper()
	user := &User{
		Username: username,
		Password: "test-password",
		Status:   1,
		AffCode:  "aff-" + username,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func createTaskQueryTask(t *testing.T, taskID string, userID int, modelName string) {
	t.Helper()
	insertTask(t, &Task{
		TaskID: taskID,
		UserId: userID,
		Status: TaskStatusInProgress,
		Properties: Properties{
			OriginModelName: modelName,
		},
	})
}

func TestTaskAdminQueryFiltersByUsernameAndModel(t *testing.T) {
	truncateTables(t)
	alpha := createTaskQueryUser(t, "alpha")
	beta := createTaskQueryUser(t, "beta")
	createTaskQueryTask(t, "task_alpha_target", alpha.Id, "minimax-h3")
	createTaskQueryTask(t, "task_alpha_other", alpha.Id, "hailuo-2.3")
	createTaskQueryTask(t, "task_beta_target", beta.Id, "minimax-h3")

	params := SyncTaskQueryParams{
		Username:  "alpha",
		ModelName: "minimax-h3",
	}
	tasks := TaskGetAllTasks(0, 20, params)

	require.Len(t, tasks, 1)
	require.Equal(t, "task_alpha_target", tasks[0].TaskID)
	require.EqualValues(t, 1, TaskCountAllTasks(params))
}

func TestTaskAdminQueryFiltersByUserIDAndModel(t *testing.T) {
	truncateTables(t)
	alpha := createTaskQueryUser(t, "alpha")
	beta := createTaskQueryUser(t, "beta")
	createTaskQueryTask(t, "task_alpha_target", alpha.Id, "minimax-h3")
	createTaskQueryTask(t, "task_beta_target", beta.Id, "minimax-h3")

	params := SyncTaskQueryParams{
		UserID:    strconv.Itoa(beta.Id),
		ModelName: "minimax-h3",
	}
	tasks := TaskGetAllTasks(0, 20, params)

	require.Len(t, tasks, 1)
	require.Equal(t, "task_beta_target", tasks[0].TaskID)
	require.EqualValues(t, 1, TaskCountAllTasks(params))
}

func TestTaskUserQueryFiltersByModel(t *testing.T) {
	truncateTables(t)
	alpha := createTaskQueryUser(t, "alpha")
	createTaskQueryTask(t, "task_alpha_target", alpha.Id, "minimax-h3")
	createTaskQueryTask(t, "task_alpha_other", alpha.Id, "hailuo-2.3")

	params := SyncTaskQueryParams{ModelName: "hailuo-2.3"}
	tasks := TaskGetAllUserTask(alpha.Id, 0, 20, params)

	require.Len(t, tasks, 1)
	require.Equal(t, "task_alpha_other", tasks[0].TaskID)
	require.EqualValues(t, 1, TaskCountAllUserTask(alpha.Id, params))
}
