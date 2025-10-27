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
			got, err := getAccountIDFromToken(tt.args.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAccountIDFromToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getAccountIDFromToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}
