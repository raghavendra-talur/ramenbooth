package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ocmworkv1 "github.com/open-cluster-management/api/work/v1"
	ramen "github.com/ramendr/ramen/api/v1alpha1"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	hub string
	dr1 string
	dr2 string
)

func init() {
	flag.StringVar(&hub, "hub", "", "path to hub kubeconfig")
	flag.StringVar(&dr1, "dr1", "", "path to dr1 kubeconfig")
	flag.StringVar(&dr2, "dr2", "", "path to dr2 kubeconfig")
	flag.Parse()
}

type clusterInfo struct {
	name           string
	status         string
	context        string
	namespaces     []string
	DRPCs          []string
	hub            bool
	managedcluster bool
}

func newClusterInfo(name, context string, hub, managedcluster bool) clusterInfo {
	return clusterInfo{
		name:           name,
		status:         "Unknown",
		context:        context,
		hub:            hub,
		managedcluster: managedcluster,
	}
}

type model struct {
	clusters []clusterInfo
	cursor   int
	width    int
	height   int
	ticker   *time.Ticker
}

func initialModel() model {
	var m model
	var clusters []clusterInfo

	clusters = append(clusters, newClusterInfo("Hub", hub, true, false))
	clusters = append(clusters, newClusterInfo("DR1", dr1, false, true))
	clusters = append(clusters, newClusterInfo("DR2", dr2, false, true))

	m.clusters = clusters

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(updateClustersData(&m.clusters), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return t
	})
}

func fetchClusterClient(kubeconfig string) client.Client {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil
	}

	kclient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil
	}

	return kclient
}

func updateClusterData(c *clusterInfo) {
	kclient := fetchClusterClient(c.context)
	if kclient == nil {
		c.status = "Error"
		c.namespaces = []string{}
	}

	c.status = getClusterStatus(c, kclient)
	c.namespaces = getRamenNamespaces(c, kclient)
	c.DRPCs = getDRPCs(c, kclient)
}

func updateClustersData(clusters *[]clusterInfo) tea.Cmd {
	return func() tea.Msg {
		for i := range *clusters {
			updateClusterData(&(*clusters)[i])
		}
		return clusters
	}
}

func filterRamenNamespaces(namespaces *corev1.NamespaceList) []string {
	var filtered []string
	for _, ns := range namespaces.Items {
		if ns.Name == "ramen-system" ||
			ns.Name == "ramen-ops" ||
			ns.Name == "openshift-operators" ||
			ns.Name == "openshift-dr-system" ||
			ns.Name == "openshift-dr-ops" {
			filtered = append(filtered, ns.Name)
		}
	}
	return filtered
}

func getNamespaces(kclient client.Client) *corev1.NamespaceList {
	namespaces := &corev1.NamespaceList{}
	err := kclient.List(context.Background(), namespaces, &client.ListOptions{})
	if err != nil {
		return &corev1.NamespaceList{}
	}

	return namespaces
}

func getRamenNamespaces(c *clusterInfo, kclient client.Client) []string {
	ns := getNamespaces(kclient)
	return filterRamenNamespaces(ns)
}

func getDRPCs(c *clusterInfo, kclient client.Client) []string {
	if !c.hub {
		return []string{}
	}

	drpcs := &ramen.DRPlacementControlList{}
	err := kclient.List(context.Background(), drpcs, &client.ListOptions{})
	if err != nil {
		return []string{}
	}

	var drpcNames []string
	for _, drpc := range drpcs.Items {
		drpcNames = append(drpcNames, drpc.Name)
	}

	return drpcNames
}

func getClusterStatus(c *clusterInfo, kclient client.Client) string {
	nodes := &corev1.NodeList{}
	err := kclient.List(context.Background(), nodes, &client.ListOptions{})
	if err != nil {
		return "Error"
	}

	return "Healthy"
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case []clusterInfo:
		m.clusters = msg

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.clusters)-1 {
				m.cursor++
			}
		}
	case time.Time:
		return m, tea.Batch(updateClustersData(&m.clusters), tickCmd())
	}
	return m, nil
}

func getHubStyle(cluster *clusterInfo, width, height int) string {
	var msg []string

	msg = append(msg, fmt.Sprintf("%s\n\n", cluster.name))
	msg = append(msg, fmt.Sprintf("Status: %s\n", cluster.status))
	msg = append(msg, fmt.Sprintf("Namespaces: %s\n", strings.Join(cluster.namespaces, ",")))
	msg = append(msg, fmt.Sprintf("DRPCs: %s\n", strings.Join(cluster.DRPCs, ",")))
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(width).
		Height(height).
		Align(lipgloss.Left).
		SetString(msg...)

	return style.Render()
}

func getManagedClusterStyle(cluster *clusterInfo, width, height int) string {
	var msg []string
	msg = append(msg, fmt.Sprintf("%s\n\n", cluster.name))
	msg = append(msg, fmt.Sprintf("Status: %s\n", cluster.status))
	msg = append(msg, fmt.Sprintf("Namespaces: %s\n", cluster.namespaces))
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true).
		Width(width).
		Height(height).
		Align(lipgloss.Left).
		SetString(msg...)

	return style.Render()
}

func (m model) View() string {
	var hubStyle string
	var dr1Style string
	var dr2Style string

	hubStyle = getHubStyle(&m.clusters[0], m.width, m.height/3)
	dr1Style = getManagedClusterStyle(&m.clusters[1], m.width/2, m.height/2)
	dr2Style = getManagedClusterStyle(&m.clusters[2], m.width/2, m.height/2)

	// Render the final view with the new layout
	s := lipgloss.JoinVertical(
		lipgloss.Top,
		hubStyle,
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			dr1Style,
			dr2Style))
	s += "\n\nPress q to quit.\n"

	return s
}

func addSchemes() {
	err := ocmworkv1.AddToScheme(scheme.Scheme)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = ramen.AddToScheme(scheme.Scheme)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// err = ocmclv1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = plrv1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = viewv1beta1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = cpcv1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = gppv1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = Recipe.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = volrep.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = volsyncv1alpha1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = snapv1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())
	// Expect(velero.AddToScheme(scheme.Scheme)).To(Succeed())

	// err = clrapiv1beta1.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())

	// err = argocdv1alpha1hack.AddToScheme(scheme.Scheme)
	// Expect(err).NotTo(HaveOccurred())
}

func main() {
	testLogger := zap.New(zap.UseFlagOptions(&zap.Options{
		Development: true,
		DestWriter:  os.Stdout,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}))
	logf.SetLogger(testLogger)
	addSchemes()
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
