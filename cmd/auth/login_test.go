package auth

import "testing"

func Test_getAccountIDFromToken(t *testing.T) {
	type args struct {
		token string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "test",
			args: args{
				token: "pat.AccountID.Random.Random",
			},
			want:    "AccountID",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetAccountIDFromToken(tt.args.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAccountIDFromToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetAccountIDFromToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}
