package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientConnectShutdown(t *testing.T) {
	require := require.New(t)

	client, err := New("testdata/client.toml", "gor")
	require.NoError(err)

	err = client.Start()
	require.NoError(err)

	<-client.session.EventSink

	client.Shutdown()
	client.Wait()

}

func TestClientSendTransaction(t *testing.T) {
	require := require.New(t)

	client, err := New("/tmp/meson-current/client.toml", "gor")
	require.NoError(err)
	require.NoError(client.Start())

	description, err := client.session.GetService("gor")
	require.NoError(err)
	// Notice that the service returns a + sign
	require.True(description.Name == "+gor")
	require.True(description.Provider == client.cfg.Panda.Provider)

	rtBlob := "0xf8640c843b9aca0083030d409400b1c66f34d680cb8bf82c64dcc1f39be5d6e77501802da03b274f8e63ce753e1ccdd03ac2d5e2595ef605335ed4962fe058eb667dbf9e6ba07c91420f9cb9805b18c6f25f84e530b35fca9eb45e4c3f6e6d624f53a3a76c40"
	chainID := 5
	ticker := "gor"
	reply, err := client.SendRawTransaction(&rtBlob, &chainID, &ticker)
	require.NoError(err)

	expected := []byte(`{"Message":"success","StatusCode":0,"Version":0}`)
	// The reply comes with zero padding on the right.
	// We only care about the contents of the message for now therefore
	// we trim the trailing zeroes by the length of the value we expect
	trimmedReply := reply[:len(expected)]
	require.EqualValues(expected, trimmedReply)

	client.Shutdown()
	client.Wait()
}
